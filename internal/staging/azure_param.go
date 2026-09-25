package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version"
)

// AzureParamStrategy implements the staging strategies for Azure App
// Configuration. App Configuration is UNVERSIONED, so:
//
//   - There are no version specifiers: version.AzureAppConfiguration takes the whole
//     argument as the key, since '#', ':' and '~' are legal key characters.
//   - Conflict detection is disabled (last-write-wins): FetchLastModified and
//     the edit base time return zero, so apply never reports a modified-after
//     conflict. Apply overwrites unconditionally. FetchLastModified still
//     reports a missing setting as not found, for the delete existence check.
//   - Tag mutation is supported (azappconfig/v2 GET-merge-PUT + ETag): ApplyTags
//     forwards TagEntry.Add/Remove to the store's Tag/Untag.
//
// A nil store yields a parser-only strategy (ParseName/ParseSpec).
type AzureParamStrategy struct {
	store provider.Store
}

// NewAzureParamStrategy creates an Azure App Configuration staging
// strategy over the given provider store. A nil store is allowed for
// parser-only use.
func NewAzureParamStrategy(store provider.Store) *AzureParamStrategy {
	return &AzureParamStrategy{store: store}
}

// Service returns the service type.
func (s *AzureParamStrategy) Service() Service { return ServiceParam }

// ServiceName returns the user-friendly service name.
func (s *AzureParamStrategy) ServiceName() string { return "App Configuration" }

// ItemName returns the item name for messages.
func (s *AzureParamStrategy) ItemName() string { return "setting" }

// HasDeleteOptions returns false: App Configuration has no delete options.
func (s *AzureParamStrategy) HasDeleteOptions() bool { return false }

// Apply applies a staged operation to Azure App Configuration.
func (s *AzureParamStrategy) Apply(ctx context.Context, name string, entry Entry) error {
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

func (s *AzureParamStrategy) applyCreate(ctx context.Context, name string, entry Entry) error {
	if _, err := s.store.Create(ctx, name, lo.FromPtr(entry.Value), domain.ValueTypePlaintext, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to create setting: %w", err)
	}

	return nil
}

func (s *AzureParamStrategy) applyUpdate(ctx context.Context, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// Last-write-wins: Put overwrites the current value unconditionally.
	if _, err := s.store.Put(ctx, name, *entry.Value, domain.ValueTypePlaintext, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to update setting: %w", err)
	}

	return nil
}

func (s *AzureParamStrategy) applyDelete(ctx context.Context, name string) error {
	if err := s.store.Delete(ctx, name); err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("failed to delete setting: %w", err)
	}

	return nil
}

// ApplyTags applies staged tag changes to App Configuration: TagEntry.Add via
// the store's Tag and TagEntry.Remove via Untag (each a GET-merge-PUT under the
// scope's namespace label). Additions are applied before removals.
func (s *AzureParamStrategy) ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error {
	if len(tagEntry.Add) > 0 {
		if err := s.store.Tag(ctx, name, tagEntry.Add); err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	if tagEntry.Remove.Len() > 0 {
		if err := s.store.Untag(ctx, name, tagEntry.Remove.Values()); err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	return nil
}

// FetchLastModified reports whether the setting exists, but never a
// modification time: App Configuration staging uses last-write-wins, so an
// existing setting yields a zero time and no modified-after conflict is ever
// reported. A missing setting yields a *ResourceNotFoundError, so the delete
// use case refuses to stage a delete of a setting that does not exist (and a
// staged create is still checked against a setting created since).
func (s *AzureParamStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	if _, err := s.store.Get(ctx, name, provider.VersionRef{}); err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return time.Time{}, &ResourceNotFoundError{Err: err}
		}

		return time.Time{}, fmt.Errorf("failed to get setting: %w", err)
	}

	return time.Time{}, nil
}

// FetchCurrent fetches the current value from App Configuration for diffing.
// App Configuration is unversioned, so the identifier is empty.
func (s *AzureParamStrategy) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		return nil, err
	}

	return &FetchResult{Value: entry.Value}, nil
}

// FetchCurrentTags fetches the setting's current tags so stage diff can show the
// current value of a removed tag. A missing setting or a setting with no tags
// yields nil.
func (s *AzureParamStrategy) FetchCurrentTags(ctx context.Context, name string) (map[string]string, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return nil, nil //nolint:nilnil // intentional: no tags for a non-existent setting
		}

		return nil, fmt.Errorf("failed to get setting: %w", err)
	}

	if len(entry.Tags) == 0 {
		return nil, nil //nolint:nilnil // intentional: setting exists but has no tags
	}

	tags := make(map[string]string, len(entry.Tags))
	for _, tag := range entry.Tags {
		tags[tag.Key] = tag.Value
	}

	return tags, nil
}

// ParseName parses and validates a name. App Configuration is unversioned, so
// the entire argument is the key (':' / '#' / '~' are legal key characters).
func (s *AzureParamStrategy) ParseName(input string) (string, error) {
	spec, err := version.AzureAppConfiguration.Parse(input)
	if err != nil {
		return "", err
	}

	return spec.Name, nil
}

// FetchCurrentValue fetches the current value for editing. LastModified is left
// zero so the edit flow records no conflict base (last-write-wins).
func (s *AzureParamStrategy) FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return nil, &ResourceNotFoundError{Err: err}
		}

		return nil, err
	}

	return &EditFetchResult{Value: entry.Value}, nil
}

// ParseSpec parses a name for reset. App Configuration is unversioned, so a
// version is never present; the entire argument is the key.
func (s *AzureParamStrategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := version.AzureAppConfiguration.Parse(input)
	if err != nil {
		return "", false, err
	}

	return spec.Name, false, nil
}

// FetchVersion fetches the current value. App Configuration is unversioned and
// the entire argument is the key, so this only ever resolves the current value.
func (s *AzureParamStrategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := version.AzureAppConfiguration.Parse(input)
	if err != nil {
		return "", "", err
	}

	entry, err := s.store.Get(ctx, spec.Name, provider.VersionRef{})
	if err != nil {
		return "", "", err
	}

	return entry.Value, "current", nil
}

// AzureParamParserFactory yields a parser-only strategy.
func AzureParamParserFactory() Parser {
	return NewAzureParamStrategy(nil)
}
