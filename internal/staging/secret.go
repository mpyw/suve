package staging

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	awssecret "github.com/mpyw/suve/internal/provider/aws/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// SecretStrategy implements ServiceStrategy for Secrets Manager. It is backed by
// a provider.Store rather than an AWS SDK client, so it carries no cloud SDK
// dependency of its own. A nil store yields a parser-only strategy.
type SecretStrategy struct {
	store provider.Store
}

// NewSecretStrategy creates a new Secrets Manager strategy over the given
// provider store. A nil store is allowed for parser-only use.
func NewSecretStrategy(store provider.Store) *SecretStrategy {
	return &SecretStrategy{store: store}
}

// Service returns the service type.
func (s *SecretStrategy) Service() Service {
	return ServiceSecret
}

// ServiceName returns the user-friendly service name.
func (s *SecretStrategy) ServiceName() string {
	return "Secrets Manager"
}

// ItemName returns the item name for messages.
func (s *SecretStrategy) ItemName() string {
	return itemNameSecret
}

// HasDeleteOptions returns true as Secrets Manager has delete options.
func (s *SecretStrategy) HasDeleteOptions() bool {
	return true
}

// Apply applies a staged operation to Secrets Manager.
func (s *SecretStrategy) Apply(ctx context.Context, name string, entry Entry) error {
	switch entry.Operation {
	case OperationCreate:
		return s.applyCreate(ctx, name, entry)
	case OperationUpdate:
		return s.applyUpdate(ctx, name, entry)
	case OperationDelete:
		return s.applyDelete(ctx, name, entry)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *SecretStrategy) applyCreate(ctx context.Context, name string, entry Entry) error {
	if _, err := s.store.Create(ctx, name, lo.FromPtr(entry.Value), domain.ValueTypeSecret, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

func (s *SecretStrategy) applyUpdate(ctx context.Context, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// Put overwrites the existing secret with a new version and, when provided,
	// updates the description in the same operation.
	if _, err := s.store.Put(ctx, name, *entry.Value, domain.ValueTypeSecret, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	return nil
}

func (s *SecretStrategy) applyDelete(ctx context.Context, name string, entry Entry) error {
	opts := deleteOptions(entry.DeleteOptions)

	if err := s.store.Delete(ctx, name, opts...); err != nil {
		// Already deleted is considered success.
		if errors.Is(err, provider.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// deleteOptions translates staged delete options into provider delete options.
func deleteOptions(o *DeleteOptions) []provider.DeleteOption {
	if o == nil {
		return nil
	}

	switch {
	case o.Force:
		return []provider.DeleteOption{awssecret.ForceDelete{}}
	case o.RecoveryWindow > 0:
		return []provider.DeleteOption{awssecret.RecoveryWindow{Days: int64(o.RecoveryWindow)}}
	default:
		return nil
	}
}

// ApplyTags applies staged tag changes to Secrets Manager.
func (s *SecretStrategy) ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error {
	if len(tagEntry.Add) > 0 {
		if err := s.store.Tag(ctx, name, tagEntry.Add); err != nil {
			return err
		}
	}

	if tagEntry.Remove.Len() > 0 {
		if err := s.store.Untag(ctx, name, tagEntry.Remove.Values()); err != nil {
			return err
		}
	}

	return nil
}

// FetchLastModified returns the last modified time of the secret. It returns a
// *ResourceNotFoundError when the secret does not exist, so callers can tell
// "missing" apart from "exists but has no modification time" (the latter returns
// a zero time with a nil error).
func (s *SecretStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return time.Time{}, &ResourceNotFoundError{Err: err}
		}

		return time.Time{}, fmt.Errorf("failed to get secret: %w", err)
	}

	if entry.Modified != nil {
		return *entry.Modified, nil
	}

	return time.Time{}, nil
}

// FetchCurrent fetches the current value from Secrets Manager for diffing.
func (s *SecretStrategy) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		return nil, err
	}

	return &FetchResult{
		Value:      entry.Value,
		Identifier: "#" + secretversion.TruncateVersionID(entry.Version.ID),
	}, nil
}

// FetchCurrentTags fetches the current tags from Secrets Manager.
func (s *SecretStrategy) FetchCurrentTags(ctx context.Context, name string) (map[string]string, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		// Secret not found - return nil (no tags available)
		if errors.Is(err, provider.ErrNotFound) {
			return nil, nil //nolint:nilnil // intentional: no tags for non-existent resource
		}

		return nil, fmt.Errorf("failed to describe secret: %w", err)
	}

	if len(entry.Tags) == 0 {
		return nil, nil //nolint:nilnil // intentional: resource exists but has no tags
	}

	tags := make(map[string]string, len(entry.Tags))
	for _, tag := range entry.Tags {
		tags[tag.Key] = tag.Value
	}

	return tags, nil
}

// ParseName parses and validates a name for editing.
func (s *SecretStrategy) ParseName(input string) (string, error) {
	spec, err := secretversion.Parse(input)
	if err != nil {
		return "", err
	}

	if spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0 {
		return "", fmt.Errorf("secret name must not contain a version specifier")
	}

	return spec.Name, nil
}

// FetchCurrentValue fetches the current value from Secrets Manager for editing.
// Returns *ResourceNotFoundError if the secret doesn't exist.
func (s *SecretStrategy) FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return nil, &ResourceNotFoundError{Err: err}
		}

		return nil, err
	}

	result := &EditFetchResult{
		Value: entry.Value,
	}

	if entry.Modified != nil {
		result.LastModified = *entry.Modified
	}

	return result, nil
}

// ParseSpec parses a version spec string for reset.
func (s *SecretStrategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := secretversion.Parse(input)
	if err != nil {
		return "", false, err
	}

	hasVersion = spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0

	return spec.Name, hasVersion, nil
}

// FetchVersion fetches the value for a specific version.
func (s *SecretStrategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := secretversion.Parse(input)
	if err != nil {
		return "", "", err
	}

	ref, err := s.store.Resolve(ctx, spec.Name, secretSpecSuffix(spec))
	if err != nil {
		return "", "", err
	}

	entry, err := s.store.Get(ctx, spec.Name, ref)
	if err != nil {
		return "", "", err
	}

	return entry.Value, "#" + secretversion.TruncateVersionID(entry.Version.ID), nil
}

// secretSpecSuffix reconstructs the version-spec suffix (the part after the name)
// so that name+suffix re-parses to an equivalent spec, as provider.Reader.Resolve expects.
func secretSpecSuffix(spec *secretversion.Spec) string {
	var b strings.Builder

	switch {
	case spec.Absolute.ID != nil:
		b.WriteString("#")
		b.WriteString(*spec.Absolute.ID)
	case spec.Absolute.Label != nil:
		b.WriteString(":")
		b.WriteString(*spec.Absolute.Label)
	}

	if spec.Shift > 0 {
		b.WriteString("~")
		b.WriteString(strconv.Itoa(spec.Shift))
	}

	return b.String()
}

// SecretParserFactory creates a Parser without provider access.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func SecretParserFactory() Parser {
	return NewSecretStrategy(nil)
}
