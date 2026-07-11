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
	"github.com/mpyw/suve/internal/version/gcloudversion"
)

// GoogleCloudSecretStrategy implements the staging strategies for Google Cloud Secret
// Manager. Like the AWS strategies it is backed by a provider.Store and carries
// no cloud SDK dependency. It differs from the AWSSecretStrategy in three
// provider-specific ways:
//
//   - Versions are immutable integers, parsed with gcloudversion (#N, ~SHIFT); a
//     staged "edit" applies as a new version via Put.
//   - There are no delete options (no force / recovery window), so
//     HasDeleteOptions reports false and Delete ignores staged DeleteOptions.
//   - There are no staging labels (:LABEL).
//
// A nil store yields a parser-only strategy (ParseName/ParseSpec).
type GoogleCloudSecretStrategy struct {
	store provider.Store
}

// NewGoogleCloudSecretStrategy creates a Google Cloud Secret Manager staging strategy
// over the given provider store. A nil store is allowed for parser-only use.
func NewGoogleCloudSecretStrategy(store provider.Store) *GoogleCloudSecretStrategy {
	return &GoogleCloudSecretStrategy{store: store}
}

// Service returns the service type.
func (s *GoogleCloudSecretStrategy) Service() Service {
	return ServiceSecret
}

// ServiceName returns the user-friendly service name.
func (s *GoogleCloudSecretStrategy) ServiceName() string {
	return "Secret Manager"
}

// ItemName returns the item name for messages.
func (s *GoogleCloudSecretStrategy) ItemName() string {
	return itemNameSecret
}

// HasDeleteOptions returns false: Google Cloud Secret Manager has no force /
// recovery-window delete options.
func (s *GoogleCloudSecretStrategy) HasDeleteOptions() bool {
	return false
}

// Apply applies a staged operation to Google Cloud Secret Manager.
func (s *GoogleCloudSecretStrategy) Apply(ctx context.Context, name string, entry Entry) error {
	switch entry.Operation {
	case OperationCreate:
		return s.applyCreate(ctx, name, entry)
	case OperationUpdate:
		return s.applyUpdate(ctx, name, entry)
	case OperationDelete:
		return s.applyDelete(ctx, name)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *GoogleCloudSecretStrategy) applyCreate(ctx context.Context, name string, entry Entry) error {
	if _, err := s.store.Create(ctx, name, lo.FromPtr(entry.Value), domain.ValueTypeSecret, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

func (s *GoogleCloudSecretStrategy) applyUpdate(ctx context.Context, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// Secret Manager versions are immutable: Put adds a new version.
	if _, err := s.store.Put(ctx, name, *entry.Value, domain.ValueTypeSecret, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	return nil
}

func (s *GoogleCloudSecretStrategy) applyDelete(ctx context.Context, name string) error {
	if err := s.store.Delete(ctx, name); err != nil {
		// Already deleted is considered success.
		if errors.Is(err, provider.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// ApplyTags applies staged tag (label) changes to Google Cloud Secret Manager.
func (s *GoogleCloudSecretStrategy) ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error {
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
func (s *GoogleCloudSecretStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
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

// FetchCurrent fetches the current value from Secret Manager for diffing.
func (s *GoogleCloudSecretStrategy) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		return nil, err
	}

	return &FetchResult{
		Value:      entry.Value,
		Identifier: "#" + entry.Version.ID,
	}, nil
}

// FetchCurrentTags fetches the current labels from Secret Manager.
func (s *GoogleCloudSecretStrategy) FetchCurrentTags(ctx context.Context, name string) (map[string]string, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return nil, nil //nolint:nilnil // intentional: no tags for non-existent resource
		}

		return nil, fmt.Errorf("failed to get secret: %w", err)
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

// ParseName parses and validates a name for editing (no version specifier).
func (s *GoogleCloudSecretStrategy) ParseName(input string) (string, error) {
	spec, err := gcloudversion.Parse(input)
	if err != nil {
		return "", err
	}

	if spec.Absolute.Version != nil || spec.Shift > 0 {
		return "", fmt.Errorf("secret name must not contain a version specifier")
	}

	return spec.Name, nil
}

// FetchCurrentValue fetches the current value from Secret Manager for editing.
// Returns *ResourceNotFoundError if the secret doesn't exist.
func (s *GoogleCloudSecretStrategy) FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error) {
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
func (s *GoogleCloudSecretStrategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := gcloudversion.Parse(input)
	if err != nil {
		return "", false, err
	}

	hasVersion = spec.Absolute.Version != nil || spec.Shift > 0

	return spec.Name, hasVersion, nil
}

// FetchVersion fetches the value for a specific version.
func (s *GoogleCloudSecretStrategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := gcloudversion.Parse(input)
	if err != nil {
		return "", "", err
	}

	ref, err := s.store.Resolve(ctx, spec.Name, gcloudSecretSpecSuffix(spec))
	if err != nil {
		return "", "", err
	}

	entry, err := s.store.Get(ctx, spec.Name, ref)
	if err != nil {
		return "", "", err
	}

	return entry.Value, "#" + entry.Version.ID, nil
}

// gcloudSecretSpecSuffix reconstructs the version-spec suffix (the part after the
// name) so that name+suffix re-parses to an equivalent spec, as
// provider.Reader.Resolve expects.
func gcloudSecretSpecSuffix(spec *gcloudversion.Spec) string {
	var b strings.Builder

	if spec.Absolute.Version != nil {
		b.WriteString("#")
		b.WriteString(strconv.FormatInt(*spec.Absolute.Version, 10))
	}

	if spec.Shift > 0 {
		b.WriteString("~")
		b.WriteString(strconv.Itoa(spec.Shift))
	}

	return b.String()
}

// GoogleCloudSecretParserFactory creates a Parser without provider access, for
// operations that don't need Google Cloud access (e.g. status, parsing).
func GoogleCloudSecretParserFactory() Parser {
	return NewGoogleCloudSecretStrategy(nil)
}
