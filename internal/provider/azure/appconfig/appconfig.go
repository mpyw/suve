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
//   - The App Configuration label axis is exposed as suve's "namespace" (see
//     the aznamespace subpackage): List forwards the raw value as a LabelFilter;
//     single-key ops decode it to one literal label; empty is the null/default
//     namespace. A label is never exposed as a version.
//   - App Configuration's PUT replaces the whole key-value, so tags must always
//     be re-sent or they are cleared. Value writes (Put) therefore GET the
//     current tags and re-send them; Tag/Untag are GET-merge-PUT with an
//     OnlyIfUnchanged (ETag) precondition and a small retry on a 412 conflict.
//     This is unblocked by azappconfig/v2, whose SetSettingOptions carries Tags
//     (map[string]*string) and OnlyIfUnchanged.
package appconfig

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"sort"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
)

// tagWriteMaxAttempts bounds the GET-merge-PUT retry loop used by Tag/Untag when
// a concurrent writer changes the setting between the GET and the conditional
// PUT (a 412). After this many conflicting attempts the error is surfaced.
const tagWriteMaxAttempts = 3

// Client is the narrow App Configuration surface this adapter needs. Single-key
// methods take a resolved literal label ("" = the null/default label); the list
// method takes a LabelFilter. SetSetting additionally carries the tags to write
// (App Config's PUT replaces the whole key-value, so tags are always re-sent)
// and an optional ETag precondition (nil = unconditional). The list method
// returns a drained slice rather than the SDK's pager so tests can mock the
// interface trivially; the production adapter (see Wrap) confines the pager
// draining and the concrete *azappconfig.Client to this package.
type Client interface {
	GetSetting(ctx context.Context, key, label string) (azappconfig.GetSettingResponse, error)
	SetSetting(
		ctx context.Context, key, value, label string, tags map[string]*string, etag *azcore.ETag,
	) (azappconfig.SetSettingResponse, error)
	AddSetting(ctx context.Context, key, value, label string) (azappconfig.AddSettingResponse, error)
	DeleteSetting(ctx context.Context, key, label string) (azappconfig.DeleteSettingResponse, error)
	ListSettings(ctx context.Context, filter string) ([]azappconfig.Setting, error)
}

// Store is the App Configuration implementation of provider.Store. It implements
// neither Restorer nor Describer.
type Store struct {
	client Client
	// namespace is the raw --namespace value selected for this store (the axis
	// Azure calls a "label"). It is interpreted per operation: a LabelFilter for
	// List, a decoded single literal label for single-key ops. Empty is the
	// null (default) namespace.
	namespace string
}

// Compile-time assertion that Store implements the provider contract.
var _ provider.Store = (*Store)(nil)

// New builds a Store backed by the given client, scoped to the given raw
// namespace value (empty = the null/default namespace).
func New(client Client, namespace string) *Store {
	return &Store{client: client, namespace: namespace}
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

	// Resolve precedes every single-item read; reject a namespace value that
	// names all/multiple namespaces here so the usage error surfaces early.
	if _, err := aznamespace.Literal(s.namespace); err != nil {
		return provider.VersionRef{}, err
	}

	return provider.NewVersionRef(""), nil
}

// Get retrieves the setting's current value and maps it to a domain.Entry. Type
// is always plaintext; Version is left empty (App Configuration has no
// versions); the setting's tags become Tags.
func (s *Store) Get(ctx context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
	label, err := aznamespace.Literal(s.namespace)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.GetSetting(ctx, name, label)
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

// List returns the distinct key names visible under the selected namespace
// filter, sorted. The raw --namespace value is forwarded as an App
// Configuration LabelFilter (empty -> the null-label filter); its `*`/`,`/`\`
// grammar is honored natively by the service.
func (s *Store) List(ctx context.Context) ([]string, error) {
	settings, err := s.client.ListSettings(ctx, aznamespace.Filter(s.namespace))
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

// KeyNamespace pairs an App Configuration setting's key with the namespace it
// lives in — the axis Azure calls a "label". An empty Namespace is the null
// (default) namespace. Value carries the setting's current value so a caller can
// display it without a second round-trip (App Configuration's list response
// already includes it). This type and ListWithNamespaces are App-Config-specific
// and are NOT part of the neutral provider seam: only a caller that has
// type-asserted the concrete App Configuration store can reach them.
type KeyNamespace struct {
	Key       string
	Namespace string
	Value     string
}

// ListWithNamespaces returns every setting across ALL namespaces (LabelFilter
// "*"), each paired with its namespace and value, sorted by key then namespace.
// Unlike List it deliberately IGNORES the store's configured namespace so a
// caller (the GUI, #425) can present a cross-namespace list and filter it
// client-side. It is an App-Config-specific extension and leaves the neutral
// provider.Reader.List contract untouched. A key that exists under several
// namespaces yields one entry per (key, namespace) pair.
func (s *Store) ListWithNamespaces(ctx context.Context) ([]KeyNamespace, error) {
	return s.listWithNamespaces(ctx, aznamespace.AllNamespacesFilter, "across namespaces")
}

// ListWithNamespacesScoped returns per-(key, namespace) rows HONORING the
// store's configured --namespace filter (empty -> the null namespace only, "*"
// -> all, "dev,prd" -> OR, "dev*" -> prefix), sorted by key then namespace. It
// is the filter-respecting sibling of ListWithNamespaces (which forces "*"):
// the CLI `param list` NAMESPACE column (#430) uses it so `param list` defaults
// to the null namespace, matching the GUI's default filter, while `--namespace
// "*"` widens to every namespace. Like ListWithNamespaces it is an
// App-Config-specific extension, not part of the neutral provider seam.
func (s *Store) ListWithNamespacesScoped(ctx context.Context) ([]KeyNamespace, error) {
	return s.listWithNamespaces(ctx, aznamespace.Filter(s.namespace), "with namespaces")
}

func (s *Store) listWithNamespaces(ctx context.Context, filter, what string) ([]KeyNamespace, error) {
	settings, err := s.client.ListSettings(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list settings %s: %w", what, err)
	}

	out := make([]KeyNamespace, 0, len(settings))
	for _, setting := range settings {
		out = append(out, KeyNamespace{
			Key:       lo.FromPtr(setting.Key),
			Namespace: lo.FromPtr(setting.Label),
			Value:     lo.FromPtr(setting.Value),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}

		return out[i].Namespace < out[j].Namespace
	})

	debug.From(ctx).Logf("azure appconfig: listWithNamespaces(filter=%q) -> %d settings\n", filter, len(out))

	return out, nil
}

// Create creates a new setting (create-only) via AddSetting and returns an empty
// version (App Configuration is unversioned). It returns a wrapped
// provider.ErrAlreadyExists if the setting already exists. The valueType and
// description are ignored. A newly created setting has no tags to preserve.
func (s *Store) Create(
	ctx context.Context, name, value string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
) (domain.Version, error) {
	label, err := aznamespace.Literal(s.namespace)
	if err != nil {
		return domain.Version{}, err
	}

	_, err = s.client.AddSetting(ctx, name, value, label)
	if err != nil {
		if isAlreadyExists(err) {
			return domain.Version{}, fmt.Errorf("%w: %s", provider.ErrAlreadyExists, name)
		}

		return domain.Version{}, fmt.Errorf("failed to create setting: %w", err)
	}

	return domain.Version{}, nil
}

// Put creates or updates a setting (upsert) via SetSetting and returns an empty
// version (App Configuration is unversioned). Because App Configuration's PUT
// replaces the whole key-value, the current tags are read first and re-sent so
// the value write does not clear them (a not-yet-existing setting has none).
// The valueType and description are ignored.
func (s *Store) Put(
	ctx context.Context, name, value string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
) (domain.Version, error) {
	label, err := aznamespace.Literal(s.namespace)
	if err != nil {
		return domain.Version{}, err
	}

	tags, err := s.currentTags(ctx, name, label)
	if err != nil {
		return domain.Version{}, err
	}

	if _, err := s.client.SetSetting(ctx, name, value, label, tags, nil); err != nil {
		return domain.Version{}, fmt.Errorf("failed to set setting: %w", err)
	}

	return domain.Version{}, nil
}

// Delete removes a setting. Provider.DeleteOptions (AWS-specific) are ignored.
func (s *Store) Delete(ctx context.Context, name string, _ ...provider.DeleteOption) error {
	label, err := aznamespace.Literal(s.namespace)
	if err != nil {
		return err
	}

	if _, err := s.client.DeleteSetting(ctx, name, label); err != nil {
		return mapError(err, name, "delete setting")
	}

	return nil
}

// Tag adds or updates the given tags on the setting, preserving the value and
// any other tags. An empty add is a no-op. See mutateTags for the GET-merge-PUT
// + ETag retry mechanics.
func (s *Store) Tag(ctx context.Context, name string, add map[string]string) error {
	if len(add) == 0 {
		return nil
	}

	return s.mutateTags(ctx, name, func(tags map[string]*string) {
		for k, v := range add {
			tags[k] = lo.ToPtr(v)
		}
	})
}

// Untag removes the given tag keys from the setting, preserving the value and
// any other tags. An empty key set is a no-op. Removing a key that is not
// present is a no-op. See mutateTags for the GET-merge-PUT + ETag retry.
func (s *Store) Untag(ctx context.Context, name string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	return s.mutateTags(ctx, name, func(tags map[string]*string) {
		for _, k := range keys {
			delete(tags, k)
		}
	})
}

// mutateTags applies merge to the setting's current tags and writes them back
// with an OnlyIfUnchanged (ETag) precondition. App Configuration has no partial
// tag update, so the whole key-value is re-PUT with the merged tags and the
// unchanged value. If a concurrent writer changes the setting between the GET
// and the PUT the service returns 412; the loop re-GETs and re-merges up to
// tagWriteMaxAttempts times before surfacing the conflict.
func (s *Store) mutateTags(ctx context.Context, name string, merge func(map[string]*string)) error {
	label, err := aznamespace.Literal(s.namespace)
	if err != nil {
		return err
	}

	var lastErr error

	for range tagWriteMaxAttempts {
		resp, err := s.client.GetSetting(ctx, name, label)
		if err != nil {
			return mapError(err, name, "get setting")
		}

		tags := cloneTags(resp.Tags)
		merge(tags)

		_, err = s.client.SetSetting(ctx, name, lo.FromPtr(resp.Value), label, tags, resp.ETag)
		if err == nil {
			return nil
		}

		if !isPreconditionFailed(err) {
			return fmt.Errorf("failed to update tags: %w", err)
		}

		lastErr = err
	}

	return fmt.Errorf(
		"failed to update tags on %q after %d attempts due to concurrent modification: %w",
		name, tagWriteMaxAttempts, lastErr,
	)
}

// currentTags returns the setting's current tags (for re-sending on a value
// write). A not-found setting has no tags to preserve, so a 404 maps to nil
// rather than an error (the ensuing Put creates the setting fresh).
func (s *Store) currentTags(ctx context.Context, name, label string) (map[string]*string, error) {
	resp, err := s.client.GetSetting(ctx, name, label)
	if err != nil {
		if isNotFound(err) {
			return nil, nil //nolint:nilnil // intentional: a missing setting has no tags to preserve
		}

		return nil, mapError(err, name, "get setting")
	}

	return resp.Tags, nil
}

// cloneTags copies a tags map so a merge does not mutate the value read off the
// GetSetting response.
func cloneTags(tags map[string]*string) map[string]*string {
	out := make(map[string]*string, len(tags))
	maps.Copy(out, tags)

	return out
}

// mapTags converts an App Configuration tags map to a sorted slice of neutral
// domain tags (sorted by key for deterministic display). Values are pointers in
// the SDK; a nil value is treated as an empty string.
func mapTags(tags map[string]*string) []domain.Tag {
	if len(tags) == 0 {
		return nil
	}

	keys := lo.Keys(tags)
	sort.Strings(keys)

	return lo.Map(keys, func(k string, _ int) domain.Tag {
		return domain.Tag{Key: k, Value: lo.FromPtr(tags[k])}
	})
}

// isNotFound reports whether err is an Azure 404 response error.
func isNotFound(err error) bool {
	var re *azcore.ResponseError

	return errors.As(err, &re) && re.StatusCode == http.StatusNotFound
}

// isPreconditionFailed reports whether err is an Azure precondition-failed (412)
// response error, returned by a conditional PUT whose ETag no longer matches.
func isPreconditionFailed(err error) bool {
	var re *azcore.ResponseError

	return errors.As(err, &re) && re.StatusCode == http.StatusPreconditionFailed
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
