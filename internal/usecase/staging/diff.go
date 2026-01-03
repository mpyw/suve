package staging

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
)

// DiffInput holds input for the diff use case.
type DiffInput struct {
	Name string // Optional: diff only this item
}

// DiffEntryType represents the type of diff entry.
type DiffEntryType int

const (
	DiffEntryNormal DiffEntryType = iota
	DiffEntryCreate
	DiffEntryAutoUnstaged
	DiffEntryWarning
)

// DiffEntry represents a single diff result.
type DiffEntry struct {
	Name          string
	Type          DiffEntryType
	Operation     staging.Operation
	AWSValue      string
	AWSIdentifier string
	StagedValue   string
	Description   *string
	Tags          map[string]string
	UntagKeys     []string
	Warning       string // For warnings like "already deleted in AWS"
}

// DiffOutput holds the result of the diff use case.
type DiffOutput struct {
	ItemName string
	Entries  []DiffEntry
}

// DiffUseCase executes diff operations.
type DiffUseCase struct {
	Strategy staging.DiffStrategy
	Store    staging.StoreReadWriter
}

// Execute runs the diff use case.
func (u *DiffUseCase) Execute(ctx context.Context, input DiffInput) (*DiffOutput, error) {
	service := u.Strategy.Service()
	itemName := u.Strategy.ItemName()

	output := &DiffOutput{
		ItemName: itemName,
	}

	// Get all staged entries for the service
	allEntries, err := u.Store.List(service)
	if err != nil {
		return nil, err
	}
	entries := allEntries[service]

	// Filter by name if specified
	if input.Name != "" {
		entry, err := u.Store.Get(service, input.Name)
		if errors.Is(err, staging.ErrNotStaged) {
			output.Entries = append(output.Entries, DiffEntry{
				Name:    input.Name,
				Type:    DiffEntryWarning,
				Warning: "not staged",
			})
			return output, nil
		}
		if err != nil {
			return nil, err
		}
		entries = map[string]staging.Entry{input.Name: *entry}
	}

	if len(entries) == 0 {
		return output, nil
	}

	// Fetch all values in parallel
	results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, name string, _ staging.Entry) (*staging.FetchResult, error) {
		return u.Strategy.FetchCurrent(ctx, name)
	})

	// Process results
	for name, entry := range entries {
		result := results[name]
		diffEntry := u.processDiffResult(name, entry, result)
		output.Entries = append(output.Entries, diffEntry)
	}

	return output, nil
}

func (u *DiffUseCase) processDiffResult(name string, entry staging.Entry, result *parallel.Result[*staging.FetchResult]) DiffEntry {
	service := u.Strategy.Service()

	if result.Err != nil {
		return u.handleFetchError(name, entry, result.Err)
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
		_ = u.Store.Unstage(service, name)
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
		Tags:          entry.Tags,
		UntagKeys:     entry.UntagKeys,
	}
}

func (u *DiffUseCase) handleFetchError(name string, entry staging.Entry, err error) DiffEntry {
	service := u.Strategy.Service()

	switch entry.Operation {
	case staging.OperationDelete:
		_ = u.Store.Unstage(service, name)
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
			Tags:        entry.Tags,
			UntagKeys:   entry.UntagKeys,
		}

	case staging.OperationUpdate:
		_ = u.Store.Unstage(service, name)
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
