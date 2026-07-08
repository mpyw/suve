package staging

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// DiffInput holds input for the diff use case.
type DiffInput struct {
	Name string // Optional: diff only this item
}

// DiffEntryType represents the type of diff entry.
type DiffEntryType int

// DiffEntryType constants representing the type of diff result.
const (
	DiffEntryNormal DiffEntryType = iota
	DiffEntryCreate
	DiffEntryAutoUnstaged
	DiffEntryWarning
)

// DiffEntry represents a single diff result for entries.
type DiffEntry struct {
	Name string
	// Namespace is the App Configuration namespace of the entry (empty for the
	// null/default namespace and every other provider).
	Namespace     string
	Type          DiffEntryType
	Operation     staging.Operation
	AWSValue      string
	AWSIdentifier string
	StagedValue   string
	Description   *string
	Warning       string // For warnings like "already deleted in AWS"
}

// DiffTagEntry represents a single diff result for tag changes.
type DiffTagEntry struct {
	Name   string
	Add    map[string]string // Tags to add or update
	Remove map[string]string // Tags to remove (key=current value from AWS)
}

// DiffOutput holds the result of the diff use case.
type DiffOutput struct {
	ItemName   string
	Entries    []DiffEntry
	TagEntries []DiffTagEntry
}

// DiffUseCase executes diff operations.
type DiffUseCase struct {
	Strategy staging.DiffStrategy
	Store    store.ReadWriteOperator
	// StrategyFor, when set, resolves the DiffStrategy for a given namespace so
	// each staged entry is diffed against a provider store scoped to its own
	// namespace (Azure App Configuration). When nil, Strategy handles every
	// entry (namespace-agnostic providers).
	StrategyFor func(namespace string) (staging.DiffStrategy, error)
}

// strategyForNamespace returns the diff strategy scoped to the given namespace,
// falling back to the single Strategy when no resolver is configured.
func (u *DiffUseCase) strategyForNamespace(namespace string) (staging.DiffStrategy, error) {
	if u.StrategyFor == nil {
		return u.Strategy, nil
	}

	return u.StrategyFor(namespace)
}

// Execute runs the diff use case.
func (u *DiffUseCase) Execute(ctx context.Context, input DiffInput) (*DiffOutput, error) {
	service := u.Strategy.Service()
	itemName := u.Strategy.ItemName()

	output := &DiffOutput{
		ItemName: itemName,
	}

	// Get all staged entries for the service
	allEntries, err := u.Store.ListEntries(ctx, service)
	if err != nil {
		return nil, err
	}

	entries := allEntries[service]

	// Get all staged tag entries for the service
	allTagEntries, err := u.Store.ListTags(ctx, service)
	if err != nil {
		return nil, err
	}

	tagEntries := allTagEntries[service]

	// Filter by name if specified. Entries/tags are keyed by the (name, namespace)
	// composite, so match on the decoded bare name — for App Configuration this
	// diffs the named setting across every namespace it is staged under.
	if input.Name != "" {
		filteredEntries := make(map[string]staging.Entry)
		filteredTags := make(map[string]staging.TagEntry)

		for key, entry := range entries {
			if name, _ := staging.SplitEntryKey(key); name == input.Name {
				filteredEntries[key] = entry
			}
		}

		for key, tagEntry := range tagEntries {
			if name, _ := staging.SplitEntryKey(key); name == input.Name {
				filteredTags[key] = tagEntry
			}
		}

		// If neither exists, return warning
		if len(filteredEntries) == 0 && len(filteredTags) == 0 {
			output.Entries = append(output.Entries, DiffEntry{
				Name:    input.Name,
				Type:    DiffEntryWarning,
				Warning: "not staged",
			})

			return output, nil
		}

		entries = filteredEntries
		tagEntries = filteredTags
	}

	// Process entries
	if len(entries) > 0 {
		// Fetch all current values in parallel, each through the strategy scoped
		// to its entry's namespace (App Configuration) or the single strategy.
		results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, key string, entry staging.Entry) (*staging.FetchResult, error) {
			name, _ := staging.SplitEntryKey(key)

			strategy, err := u.strategyForNamespace(entry.Namespace)
			if err != nil {
				return nil, err
			}

			return strategy.FetchCurrent(ctx, name)
		})

		// Process results
		for key, entry := range entries {
			name, _ := staging.SplitEntryKey(key)
			result := results[key]
			diffEntry := u.processDiffResult(ctx, name, entry, result)
			output.Entries = append(output.Entries, diffEntry)
		}
	}

	// Process tag entries - fetch current values for removed tags
	for name, tagEntry := range tagEntries {
		diffTagEntry := DiffTagEntry{
			Name: name,
			Add:  tagEntry.Add,
		}

		// Fetch current tag values for removed tags
		if tagEntry.Remove.Len() > 0 {
			currentTags, err := u.Strategy.FetchCurrentTags(ctx, name)
			if err != nil {
				// If fetch fails, use keys only (fallback to old behavior)
				diffTagEntry.Remove = make(map[string]string, tagEntry.Remove.Len())
				for key := range tagEntry.Remove {
					diffTagEntry.Remove[key] = ""
				}
			} else {
				diffTagEntry.Remove = make(map[string]string, tagEntry.Remove.Len())
				for key := range tagEntry.Remove {
					if currentTags != nil {
						diffTagEntry.Remove[key] = currentTags[key]
					} else {
						diffTagEntry.Remove[key] = ""
					}
				}
			}
		}

		output.TagEntries = append(output.TagEntries, diffTagEntry)
	}

	return output, nil
}

//nolint:lll // function parameters are descriptive for clarity
func (u *DiffUseCase) processDiffResult(ctx context.Context, name string, entry staging.Entry, result *parallel.Result[*staging.FetchResult]) DiffEntry {
	service := u.Strategy.Service()

	if result.Err != nil {
		return u.handleFetchError(ctx, name, entry, result.Err)
	}

	fetchResult := result.Value
	awsValue := fetchResult.Value
	stagedValue := lo.FromPtr(entry.Value)

	// For delete operation, staged value is empty
	if entry.Operation == staging.OperationDelete {
		stagedValue = ""
	}

	// Check if identical and auto-unstage. This never applies to a delete:
	// deleting a resource is not a no-op just because its current value happens
	// to be the empty string (legal in Azure App Configuration / Key Vault /
	// Google Cloud), so an empty-valued delete must not be silently cancelled.
	if entry.Operation != staging.OperationDelete && awsValue == stagedValue {
		_ = u.Store.UnstageEntry(ctx, service, name, entry.Namespace)

		return DiffEntry{
			Name:      name,
			Namespace: entry.Namespace,
			Type:      DiffEntryAutoUnstaged,
			Warning:   "identical to AWS current",
		}
	}

	return DiffEntry{
		Name:          name,
		Namespace:     entry.Namespace,
		Type:          DiffEntryNormal,
		Operation:     entry.Operation,
		AWSValue:      awsValue,
		AWSIdentifier: fetchResult.Identifier,
		StagedValue:   stagedValue,
		Description:   entry.Description,
	}
}

func (u *DiffUseCase) handleFetchError(ctx context.Context, name string, entry staging.Entry, err error) DiffEntry {
	service := u.Strategy.Service()

	// Only a genuine "not found" justifies auto-unstaging a staged delete or
	// update. Any other fetch error (expired credentials, throttling, a network
	// blip) must NOT discard staged work on a read-only `stage diff`: surface it
	// as a warning and leave the staged entry untouched.
	notFound := errors.Is(err, provider.ErrNotFound)

	switch entry.Operation {
	case staging.OperationDelete:
		if notFound {
			_ = u.Store.UnstageEntry(ctx, service, name, entry.Namespace)

			return DiffEntry{
				Name:      name,
				Namespace: entry.Namespace,
				Type:      DiffEntryAutoUnstaged,
				Warning:   "already deleted in AWS",
			}
		}

	case staging.OperationCreate:
		return DiffEntry{
			Name:        name,
			Namespace:   entry.Namespace,
			Type:        DiffEntryCreate,
			Operation:   entry.Operation,
			StagedValue: lo.FromPtr(entry.Value),
			Description: entry.Description,
		}

	case staging.OperationUpdate:
		if notFound {
			_ = u.Store.UnstageEntry(ctx, service, name, entry.Namespace)

			return DiffEntry{
				Name:      name,
				Namespace: entry.Namespace,
				Type:      DiffEntryAutoUnstaged,
				Warning:   "item no longer exists in AWS",
			}
		}
	}

	return DiffEntry{
		Name:      name,
		Namespace: entry.Namespace,
		Type:      DiffEntryWarning,
		Warning:   err.Error(),
	}
}
