package staging

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/parallel"
	"github.com/mpyw/suve/internal/staging"
)

// ApplyInput holds input for the apply use case.
type ApplyInput struct {
	Name            string // Optional: apply only this item
	IgnoreConflicts bool   // Skip conflict detection
}

// ApplyResultStatus represents the status of an apply operation.
type ApplyResultStatus int

const (
	ApplyResultCreated ApplyResultStatus = iota
	ApplyResultUpdated
	ApplyResultDeleted
	ApplyResultFailed
)

// ApplyEntryResult represents the result of applying a single entry.
type ApplyEntryResult struct {
	Name   string
	Status ApplyResultStatus
	Error  error
}

// ApplyTagResult represents the result of applying tag changes.
type ApplyTagResult struct {
	Name      string
	AddTags   map[string]string   // Tags that were added/updated
	RemoveTag maputil.Set[string] // Tag keys that were removed
	Error     error
}

// ApplyOutput holds the result of the apply use case.
type ApplyOutput struct {
	ServiceName string
	ItemName    string
	// Entry results
	EntryResults   []ApplyEntryResult
	EntrySucceeded int
	EntryFailed    int
	// Tag results
	TagResults   []ApplyTagResult
	TagSucceeded int
	TagFailed    int
	// Conflicts
	Conflicts []string
}

// ApplyUseCase executes apply operations.
type ApplyUseCase struct {
	Strategy staging.ApplyStrategy
	Store    staging.StoreReadWriter
}

// Execute runs the apply use case.
func (u *ApplyUseCase) Execute(ctx context.Context, input ApplyInput) (*ApplyOutput, error) {
	service := u.Strategy.Service()
	serviceName := u.Strategy.ServiceName()
	itemName := u.Strategy.ItemName()

	output := &ApplyOutput{
		ServiceName: serviceName,
		ItemName:    itemName,
	}

	// Get staged entries and tags
	stagedEntries, err := u.Store.ListEntries(ctx, service)
	if err != nil {
		return nil, err
	}
	stagedTags, err := u.Store.ListTags(ctx, service)
	if err != nil {
		return nil, err
	}

	entries := stagedEntries[service]
	tags := stagedTags[service]

	// Filter by name if specified
	if input.Name != "" {
		filteredEntries := make(map[string]staging.Entry)
		filteredTags := make(map[string]staging.TagEntry)

		if entry, exists := entries[input.Name]; exists {
			filteredEntries[input.Name] = entry
		}
		if tagEntry, exists := tags[input.Name]; exists {
			filteredTags[input.Name] = tagEntry
		}

		if len(filteredEntries) == 0 && len(filteredTags) == 0 {
			return nil, fmt.Errorf("%s %s is not staged", itemName, input.Name)
		}
		entries = filteredEntries
		tags = filteredTags
	}

	if len(entries) == 0 && len(tags) == 0 {
		return output, nil
	}

	// Check for conflicts (only for entries, as tags don't have value conflicts)
	if !input.IgnoreConflicts && len(entries) > 0 {
		conflicts := staging.CheckConflicts(ctx, u.Strategy, entries)
		if len(conflicts) > 0 {
			for name := range conflicts {
				output.Conflicts = append(output.Conflicts, name)
			}
			return output, fmt.Errorf("apply rejected: %d conflict(s) detected", len(conflicts))
		}
	}

	// Apply entries
	if len(entries) > 0 {
		u.applyEntries(ctx, service, entries, output)
	}

	// Apply tags
	if len(tags) > 0 {
		u.applyTags(ctx, service, tags, output)
	}

	// Calculate total failures
	totalFailed := output.EntryFailed + output.TagFailed
	if totalFailed > 0 {
		return output, fmt.Errorf("applied %d entries, %d tags; failed %d entries, %d tags",
			output.EntrySucceeded, output.TagSucceeded, output.EntryFailed, output.TagFailed)
	}

	return output, nil
}

func (u *ApplyUseCase) applyEntries(ctx context.Context, service staging.Service, entries map[string]staging.Entry, output *ApplyOutput) {
	// Execute apply operations in parallel
	results := parallel.ExecuteMap(ctx, entries, func(ctx context.Context, name string, entry staging.Entry) (staging.Operation, error) {
		err := u.Strategy.Apply(ctx, name, entry)
		return entry.Operation, err
	})

	// Collect results
	for name := range entries {
		result := results[name]
		resultEntry := ApplyEntryResult{
			Name: name,
		}

		if result.Err != nil {
			resultEntry.Status = ApplyResultFailed
			resultEntry.Error = result.Err
			output.EntryFailed++
		} else {
			switch result.Value {
			case staging.OperationCreate:
				resultEntry.Status = ApplyResultCreated
			case staging.OperationUpdate:
				resultEntry.Status = ApplyResultUpdated
			case staging.OperationDelete:
				resultEntry.Status = ApplyResultDeleted
			}
			// Unstage successful operations
			_ = u.Store.UnstageEntry(ctx, service, name)
			output.EntrySucceeded++
		}
		output.EntryResults = append(output.EntryResults, resultEntry)
	}
}

func (u *ApplyUseCase) applyTags(ctx context.Context, service staging.Service, tags map[string]staging.TagEntry, output *ApplyOutput) {
	// Execute tag apply operations in parallel
	results := parallel.ExecuteMap(ctx, tags, func(ctx context.Context, name string, tagEntry staging.TagEntry) (struct{}, error) {
		err := u.Strategy.ApplyTags(ctx, name, tagEntry)
		return struct{}{}, err
	})

	// Collect results
	for name, tagEntry := range tags {
		result := results[name]
		resultTag := ApplyTagResult{
			Name:      name,
			AddTags:   tagEntry.Add,
			RemoveTag: tagEntry.Remove,
		}

		if result.Err != nil {
			resultTag.Error = result.Err
			output.TagFailed++
		} else {
			// Unstage successful operations
			_ = u.Store.UnstageTag(ctx, service, name)
			output.TagSucceeded++
		}
		output.TagResults = append(output.TagResults, resultTag)
	}
}
