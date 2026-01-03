package staging

import (
	"context"
	"errors"
	"time"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
)

// TagInput holds input for the tag staging use case.
type TagInput struct {
	Name             string
	AddTags          map[string]string   // Tags to add or update
	RemoveTags       maputil.Set[string] // Tag keys to remove
	CancelAddTags    maputil.Set[string] // Cancel staged tag additions (remove from Tags only)
	CancelRemoveTags maputil.Set[string] // Cancel staged tag removals (remove from UntagKeys only)
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
		if entry.UntagKeys == nil {
			entry.UntagKeys = maputil.NewSet[string]()
		}
		for k, v := range input.AddTags {
			entry.Tags[k] = v
			// Remove from untag list if present
			entry.UntagKeys.Remove(k)
		}

		// Merge untag keys
		for k := range input.RemoveTags {
			// Remove from tags if present
			delete(entry.Tags, k)
			// Add to untag list
			entry.UntagKeys.Add(k)
		}

		// Cancel staged tag additions (remove from Tags only, don't add to UntagKeys)
		for k := range input.CancelAddTags {
			delete(entry.Tags, k)
		}

		// Cancel staged tag removals (remove from UntagKeys only, don't add to Tags)
		for k := range input.CancelRemoveTags {
			entry.UntagKeys.Remove(k)
		}

		// If entry has no meaningful content after cancellation, unstage it
		if entry.Value == nil && entry.Description == nil && len(entry.Tags) == 0 && entry.UntagKeys.Len() == 0 {
			if err := u.Store.Unstage(service, name); err != nil {
				return nil, err
			}
			return &TagOutput{Name: name}, nil
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
		if input.RemoveTags.Len() > 0 {
			entry.UntagKeys = input.RemoveTags
		}
	}

	if err := u.Store.Stage(service, name, entry); err != nil {
		return nil, err
	}

	return &TagOutput{Name: name}, nil
}
