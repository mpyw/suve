package staging

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/parallel"
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
	Name          string
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
	Add    map[string]string   // Tags to add or update
	Remove maputil.Set[string] // Tag keys to remove
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
	Store    store.ServiceReadWriter
}

// Execute runs the diff use case.
func (u *DiffUseCase) Execute(ctx context.Context, input DiffInput) (*DiffOutput, error) {
	itemName := u.Strategy.ItemName()

	output := &DiffOutput{
		ItemName: itemName,
	}

	// Get all staged entries for the service
	entries, err := u.Store.ListEntries(ctx)
	if err != nil {
		return nil, err
	}

	// Get all staged tag entries for the service
	tagEntries, err := u.Store.ListTags(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by name if specified
	if input.Name != "" {
		// Get specific entry
		entry, entryErr := u.Store.GetEntry(ctx, input.Name)
		if entryErr != nil && !errors.Is(entryErr, staging.ErrNotStaged) {
			return nil, entryErr
		}

		// Get specific tag entry
		tagEntry, tagErr := u.Store.GetTag(ctx, input.Name)
		if tagErr != nil && !errors.Is(tagErr, staging.ErrNotStaged) {
			return nil, tagErr
		}

		// If neither exists, return warning
		if entry == nil && tagEntry == nil {
			output.Entries = append(output.Entries, DiffEntry{
				Name:    input.Name,
				Type:    DiffEntryWarning,
				Warning: "not staged",
			})

			return output, nil
		}

		if entry != nil {
			entries = map[string]staging.Entry{input.Name: *entry}
		} else {
			entries = nil
		}

		if tagEntry != nil {
			tagEntries = map[string]staging.TagEntry{input.Name: *tagEntry}
		} else {
			tagEntries = nil
		}
	}

	// Process entries
	if len(entries) > 0 {
		// Fetch all values in parallel
		results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, name string, _ staging.Entry) (*staging.FetchResult, error) {
			return u.Strategy.FetchCurrent(ctx, name)
		})

		// Process results
		for name, entry := range entries {
			result := results[name]
			diffEntry := u.processDiffResult(ctx, name, entry, result)
			output.Entries = append(output.Entries, diffEntry)
		}
	}

	// Process tag entries (just list them, no comparison needed)
	for name, tagEntry := range tagEntries {
		output.TagEntries = append(output.TagEntries, DiffTagEntry{
			Name:   name,
			Add:    tagEntry.Add,
			Remove: tagEntry.Remove,
		})
	}

	return output, nil
}

//nolint:lll // function parameters are descriptive for clarity
func (u *DiffUseCase) processDiffResult(ctx context.Context, name string, entry staging.Entry, result *parallel.Result[*staging.FetchResult]) DiffEntry {
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

	// Check if identical and auto-unstage
	if awsValue == stagedValue {
		_ = u.Store.UnstageEntry(ctx, name)

		return DiffEntry{
			Name:    name,
			Type:    DiffEntryAutoUnstaged,
			Warning: "identical to AWS current",
		}
	}

	return DiffEntry{
		Name:          name,
		Type:          DiffEntryNormal,
		Operation:     entry.Operation,
		AWSValue:      awsValue,
		AWSIdentifier: fetchResult.Identifier,
		StagedValue:   stagedValue,
		Description:   entry.Description,
	}
}

func (u *DiffUseCase) handleFetchError(ctx context.Context, name string, entry staging.Entry, err error) DiffEntry {
	switch entry.Operation {
	case staging.OperationDelete:
		_ = u.Store.UnstageEntry(ctx, name)

		return DiffEntry{
			Name:    name,
			Type:    DiffEntryAutoUnstaged,
			Warning: "already deleted in AWS",
		}

	case staging.OperationCreate:
		return DiffEntry{
			Name:        name,
			Type:        DiffEntryCreate,
			Operation:   entry.Operation,
			StagedValue: lo.FromPtr(entry.Value),
			Description: entry.Description,
		}

	case staging.OperationUpdate:
		_ = u.Store.UnstageEntry(ctx, name)

		return DiffEntry{
			Name:    name,
			Type:    DiffEntryAutoUnstaged,
			Warning: "item no longer exists in AWS",
		}
	}

	return DiffEntry{
		Name:    name,
		Type:    DiffEntryWarning,
		Warning: err.Error(),
	}
}
