package staging

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
)

// TagInput holds input for the tag staging use case.
type TagInput struct {
	Name       string
	AddTags    map[string]string // Tags to add or update
	RemoveTags []string          // Tag keys to remove
}

// TagOutput holds the result of the tag staging use case.
type TagOutput struct {
	Name string
}

// TagUseCase executes tag staging operations.
type TagUseCase struct {
	Strategy staging.EditStrategy
	Store    staging.StoreReadWriter
}

// Execute runs the tag staging use case.
func (u *TagUseCase) Execute(ctx context.Context, input TagInput) (*TagOutput, error) {
	service := u.Strategy.Service()

	// Parse and validate name
	name, err := u.Strategy.ParseName(input.Name)
	if err != nil {
		return nil, err
	}

	// Get existing staged entry
	existingEntry, err := u.Store.Get(service, name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}

	var entry staging.Entry
	if existingEntry != nil {
		// Merge with existing entry
		entry = *existingEntry
		entry.StagedAt = time.Now()

		// Merge tags
		if entry.Tags == nil {
			entry.Tags = make(map[string]string)
		}
		for k, v := range input.AddTags {
			entry.Tags[k] = v
			// Remove from untag list if present
			entry.UntagKeys = lo.Without(entry.UntagKeys, k)
		}

		// Merge untag keys
		for _, k := range input.RemoveTags {
			// Remove from tags if present
			delete(entry.Tags, k)
			// Add to untag list if not already present
			if !lo.Contains(entry.UntagKeys, k) {
				entry.UntagKeys = append(entry.UntagKeys, k)
			}
		}
	} else {
		// Create new entry for tag-only change
		// Fetch base time from AWS for conflict detection
		result, err := u.Strategy.FetchCurrentValue(ctx, name)
		if err != nil {
			return nil, err
		}

		entry = staging.Entry{
			Operation: staging.OperationUpdate,
			StagedAt:  time.Now(),
		}
		if !result.LastModified.IsZero() {
			entry.BaseModifiedAt = &result.LastModified
		}

		if len(input.AddTags) > 0 {
			entry.Tags = input.AddTags
		}
		if len(input.RemoveTags) > 0 {
			entry.UntagKeys = input.RemoveTags
		}
	}

	if err := u.Store.Stage(service, name, entry); err != nil {
		return nil, err
	}

	return &TagOutput{Name: name}, nil
}
