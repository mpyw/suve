package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
)

// AzureAppConfigParamStrategy implements the staging strategies for Azure App
// Configuration. App Configuration is UNVERSIONED, so:
//
//   - Version specifiers (#VERSION, ~SHIFT, :LABEL) are rejected at parse time
//     via azureappconfigversion.
//   - Conflict detection is disabled (last-write-wins): FetchLastModified and
//     the edit base time return zero, so apply never reports a modified-after
//     conflict. Apply overwrites unconditionally.
//   - Tag mutation is supported (azappconfig/v2 GET-merge-PUT + ETag): ApplyTags
//     forwards TagEntry.Add/Remove to the store's Tag/Untag.
//
// A nil store yields a parser-only strategy (ParseName/ParseSpec).
type AzureAppConfigParamStrategy struct {
	store provider.Store
}

// NewAzureAppConfigParamStrategy creates an Azure App Configuration staging
// strategy over the given provider store. A nil store is allowed for
// parser-only use.
func NewAzureAppConfigParamStrategy(store provider.Store) *AzureAppConfigParamStrategy {
	return &AzureAppConfigParamStrategy{store: store}
}

// Service returns the service type.
func (s *AzureAppConfigParamStrategy) Service() Service { return ServiceParam }

// ServiceName returns the user-friendly service name.
func (s *AzureAppConfigParamStrategy) ServiceName() string { return "App Configuration" }

// ItemName returns the item name for messages.
func (s *AzureAppConfigParamStrategy) ItemName() string { return "setting" }

// HasDeleteOptions returns false: App Configuration has no delete options.
func (s *AzureAppConfigParamStrategy) HasDeleteOptions() bool { return false }

// Apply applies a staged operation to Azure App Configuration.
func (s *AzureAppConfigParamStrategy) Apply(ctx context.Context, name string, entry Entry) error {
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

func (s *AzureAppConfigParamStrategy) applyCreate(ctx context.Context, name string, entry Entry) error {
	if _, err := s.store.Create(ctx, name, lo.FromPtr(entry.Value), domain.ValueTypePlaintext, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to create setting: %w", err)
	}

	return nil
}

func (s *AzureAppConfigParamStrategy) applyUpdate(ctx context.Context, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// Last-write-wins: Put overwrites the current value unconditionally.
	if _, err := s.store.Put(ctx, name, *entry.Value, domain.ValueTypePlaintext, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to update setting: %w", err)
	}

	return nil
}

func (s *AzureAppConfigParamStrategy) applyDelete(ctx context.Context, name string) error {
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
func (s *AzureAppConfigParamStrategy) ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error {
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

// FetchLastModified returns a zero time with a nil error: App Configuration
// staging uses last-write-wins, so no modified-after conflict is ever reported.
// The nil error means the delete use case treats every setting as existing
// (never "not found"); apply is idempotent on a missing setting, so this is
// consistent with the last-write-wins model.
func (s *AzureAppConfigParamStrategy) FetchLastModified(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}

// FetchCurrent fetches the current value from App Configuration for diffing.
// App Configuration is unversioned, so the identifier is empty.
func (s *AzureAppConfigParamStrategy) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		return nil, err
	}

	return &FetchResult{Value: entry.Value}, nil
}

// FetchCurrentTags fetches the setting's current tags so stage diff can show the
// current value of a removed tag. A missing setting or a setting with no tags
// yields nil.
func (s *AzureAppConfigParamStrategy) FetchCurrentTags(ctx context.Context, name string) (map[string]string, error) {
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
func (s *AzureAppConfigParamStrategy) ParseName(input string) (string, error) {
	spec, err := azureappconfigversion.Parse(input)
	if err != nil {
		return "", err
	}

	return spec.Name, nil
}

// FetchCurrentValue fetches the current value for editing. LastModified is left
// zero so the edit flow records no conflict base (last-write-wins).
func (s *AzureAppConfigParamStrategy) FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error) {
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
func (s *AzureAppConfigParamStrategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := azureappconfigversion.Parse(input)
	if err != nil {
		return "", false, err
	}

	return spec.Name, false, nil
}

// FetchVersion fetches the current value. App Configuration is unversioned and
// the entire argument is the key, so this only ever resolves the current value.
func (s *AzureAppConfigParamStrategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := azureappconfigversion.Parse(input)
	if err != nil {
		return "", "", err
	}

	entry, err := s.store.Get(ctx, spec.Name, provider.VersionRef{})
	if err != nil {
		return "", "", err
	}

	return entry.Value, "current", nil
}

// AzureAppConfigParamParserFactory yields a parser-only strategy.
func AzureAppConfigParamParserFactory() Parser {
	return NewAzureAppConfigParamStrategy(nil)
}
