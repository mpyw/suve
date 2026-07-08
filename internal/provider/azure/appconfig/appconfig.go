// Package appconfig implements the provider.Store contract (Reader/Writer/Tagger)
// for Azure App Configuration, confining all App Configuration SDK types to this
// package. Spec PARSING stays generic via azureappconfigversion.Parse.
//
// Azure App Configuration is the abstraction's acid test: it has NO versioning.
// A key/label pair holds a single current value with no history. Every
// version-dependent operation therefore degrades cleanly rather than crashing:
//
//   - Resolve accepts a bare name only (any #/~/: specifier is rejected by
//     azureappconfigversion.Parse before any API call) and always returns the
//     latest ref.
//   - History returns ErrVersioningUnsupported so the generic log command
//     surfaces a clear error instead of iterating a non-existent history.
//
// Two further constraints shape the adapter:
//
//   - It addresses the default (no) App Configuration label only; the default
//     label is never exposed as a version.
//   - The azappconfig high-level SDK cannot round-trip setting tags: its
//     SetSetting/AddSetting drop tags (and a write would clear existing ones).
//     Tag/Untag therefore return ErrTagsUnsupported rather than silently losing
//     data. Reading tags (Get) works, since GetSetting returns them.
package appconfig

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
)

// ErrTagsUnsupported is returned by Tag/Untag: the azappconfig high-level SDK
// surface cannot write setting tags without clearing them, so tag mutation is
// declined rather than performed destructively.
var ErrTagsUnsupported = errors.New(
	"tag mutation is not supported by Azure App Configuration (the azappconfig SDK cannot write setting tags)",
)

// Client is the narrow App Configuration surface this adapter needs. Every
// method uses the default (no) label. The list method returns a drained slice
// rather than the SDK's pager so tests can mock the interface trivially; the
// production adapter (see Wrap) confines the pager draining and the concrete
// *azappconfig.Client to this package.
type Client interface {
	GetSetting(ctx context.Context, key string) (azappconfig.GetSettingResponse, error)
	SetSetting(ctx context.Context, key, value string) (azappconfig.SetSettingResponse, error)
	AddSetting(ctx context.Context, key, value string) (azappconfig.AddSettingResponse, error)
	DeleteSetting(ctx context.Context, key string) (azappconfig.DeleteSettingResponse, error)
	ListSettings(ctx context.Context) ([]azappconfig.Setting, error)
}

// Store is the App Configuration implementation of provider.Store. It implements
// neither Restorer nor Describer.
type Store struct {
	client Client
}

// Compile-time assertion that Store implements the provider contract.
var _ provider.Store = (*Store)(nil)

// New builds a Store backed by the given client.
func New(client Client) *Store {
	return &Store{client: client}
}

// Resolve validates that the spec carries no version specifier (App
// Configuration has none) and returns the latest ref. The parse guard rejects
// any #/~/: specifier with a clear error before any API call.
func (s *Store) Resolve(_ context.Context, _, spec string) (provider.VersionRef, error) {
	// App Configuration is unversioned: the entire argument is the key name, so
	// the caller passes no version specifier. A non-empty spec means something
	// tried to version an unversioned store, which is unsupported.
	if spec != "" {
		return provider.VersionRef{}, fmt.Errorf("%w", azureappconfigversion.ErrVersioningUnsupported)
	}

	return provider.NewVersionRef(""), nil
}

// Get retrieves the setting's current value and maps it to a domain.Entry. Type
// is always plaintext; Version is left empty (App Configuration has no
// versions); the setting's tags become Tags.
func (s *Store) Get(ctx context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
	resp, err := s.client.GetSetting(ctx, name)
	if err != nil {
		return nil, mapError(err, name, "get setting")
	}

	return &domain.Entry{
		Name:     name,
		Value:    lo.FromPtr(resp.Value),
		Type:     domain.ValueTypePlaintext,
		Version:  domain.Version{},
		Tags:     mapTags(resp.Tags),
		Modified: resp.LastModified,
	}, nil
}

// History returns ErrVersioningUnsupported: App Configuration keeps no version
// history. The generic log command surfaces this error without crashing.
func (s *Store) History(_ context.Context, _ string) ([]domain.Version, error) {
	return nil, fmt.Errorf("%w (no version history)", azureappconfigversion.ErrVersioningUnsupported)
}

// List returns the distinct key names in the store (across all labels), sorted.
func (s *Store) List(ctx context.Context) ([]string, error) {
	settings, err := s.client.ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list settings: %w", err)
	}

	seen := make(map[string]struct{}, len(settings))

	var names []string

	for _, setting := range settings {
		name := lo.FromPtr(setting.Key)
		if _, ok := seen[name]; ok {
			continue
		}

		seen[name] = struct{}{}

		names = append(names, name)
	}

	sort.Strings(names)

	// The totals make a successful-but-empty result (wrong store) visible at a
	// glance, which a bodyless HTTP log cannot.
	debug.From(ctx).Logf("azure appconfig: ListSettings -> %d settings, %d distinct keys\n", len(settings), len(names))

	return names, nil
}

// Create creates a new setting (create-only) via AddSetting and returns an empty
// version (App Configuration is unversioned). It returns a wrapped
// provider.ErrAlreadyExists if the setting already exists. The valueType and
// description are ignored.
func (s *Store) Create(
	ctx context.Context, name, value string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
) (domain.Version, error) {
	_, err := s.client.AddSetting(ctx, name, value)
	if err != nil {
		if isAlreadyExists(err) {
			return domain.Version{}, fmt.Errorf("%w: %s", provider.ErrAlreadyExists, name)
		}

		return domain.Version{}, fmt.Errorf("failed to create setting: %w", err)
	}

	return domain.Version{}, nil
}

// Put creates or updates a setting (upsert) via SetSetting and returns an empty
// version (App Configuration is unversioned). The valueType and description are
// ignored.
func (s *Store) Put(
	ctx context.Context, name, value string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
) (domain.Version, error) {
	if _, err := s.client.SetSetting(ctx, name, value); err != nil {
		return domain.Version{}, fmt.Errorf("failed to set setting: %w", err)
	}

	return domain.Version{}, nil
}

// Delete removes a setting. Provider.DeleteOptions (AWS-specific) are ignored.
func (s *Store) Delete(ctx context.Context, name string, _ ...provider.DeleteOption) error {
	if _, err := s.client.DeleteSetting(ctx, name); err != nil {
		return mapError(err, name, "delete setting")
	}

	return nil
}

// Tag declines tag mutation: the azappconfig SDK cannot write setting tags
// without clearing them (see the package doc). An empty add is a no-op.
func (s *Store) Tag(_ context.Context, _ string, add map[string]string) error {
	if len(add) == 0 {
		return nil
	}

	return ErrTagsUnsupported
}

// Untag declines tag mutation for the same reason as Tag. An empty key set is a
// no-op.
func (s *Store) Untag(_ context.Context, _ string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	return ErrTagsUnsupported
}

// mapTags converts an App Configuration tags map to a sorted slice of neutral
// domain tags (sorted by key for deterministic display).
func mapTags(tags map[string]string) []domain.Tag {
	if len(tags) == 0 {
		return nil
	}

	keys := lo.Keys(tags)
	sort.Strings(keys)

	return lo.Map(keys, func(k string, _ int) domain.Tag {
		return domain.Tag{Key: k, Value: tags[k]}
	})
}

// isNotFound reports whether err is an Azure 404 response error.
func isNotFound(err error) bool {
	var re *azcore.ResponseError

	return errors.As(err, &re) && re.StatusCode == http.StatusNotFound
}

// isAlreadyExists reports whether err is an Azure precondition-failed (412) or
// conflict (409) response error, which AddSetting returns when the setting
// already exists.
func isAlreadyExists(err error) bool {
	var re *azcore.ResponseError

	return errors.As(err, &re) &&
		(re.StatusCode == http.StatusPreconditionFailed || re.StatusCode == http.StatusConflict)
}

// mapError maps an Azure 404 to provider.ErrNotFound and otherwise wraps the
// error with the given operation description.
func mapError(err error, name, op string) error {
	if isNotFound(err) {
		return fmt.Errorf("%w: %s", provider.ErrNotFound, name)
	}

	return fmt.Errorf("failed to %s: %w", op, err)
}
