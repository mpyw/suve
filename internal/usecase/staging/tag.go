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
	Name       string
	AddTags    map[string]string   // Tags to add or update
	RemoveTags maputil.Set[string] // Tag keys to remove
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

	// Get existing staged tag entry
	existingTagEntry, err := u.Store.GetTag(service, name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}

	var tagEntry staging.TagEntry
	if existingTagEntry != nil {
		// Merge with existing tag entry
		tagEntry = *existingTagEntry
		tagEntry.StagedAt = time.Now()

		// Ensure maps are initialized
		if tagEntry.Add == nil {
			tagEntry.Add = make(map[string]string)
		}
		if tagEntry.Remove == nil {
			tagEntry.Remove = maputil.NewSet[string]()
		}

		// Process add tags
		for k, v := range input.AddTags {
			tagEntry.Add[k] = v
			// Remove from remove list if present (adding takes precedence)
			tagEntry.Remove.Remove(k)
		}

		// Process remove tags
		for k := range input.RemoveTags {
			// Remove from add list if present
			delete(tagEntry.Add, k)
			// Add to remove list
			tagEntry.Remove.Add(k)
		}

		// If tag entry has no meaningful content after merging, unstage it
		if len(tagEntry.Add) == 0 && tagEntry.Remove.Len() == 0 {
			if err := u.Store.UnstageTag(service, name); err != nil {
				return nil, err
			}
			return &TagOutput{Name: name}, nil
		}
	} else {
		// No existing tag entry - only create if there's something to stage
		if len(input.AddTags) == 0 && input.RemoveTags.Len() == 0 {
			return &TagOutput{Name: name}, nil
		}

		// Fetch base time from AWS for conflict detection
		result, err := u.Strategy.FetchCurrentValue(ctx, name)
		if err != nil {
			return nil, err
		}

		tagEntry = staging.TagEntry{
			StagedAt: time.Now(),
		}
		if !result.LastModified.IsZero() {
			tagEntry.BaseModifiedAt = &result.LastModified
		}

		if len(input.AddTags) > 0 {
			tagEntry.Add = make(map[string]string)
			for k, v := range input.AddTags {
				tagEntry.Add[k] = v
			}
		}
		if input.RemoveTags.Len() > 0 {
			// Filter out keys that are being added (add takes precedence)
			tagEntry.Remove = maputil.NewSet[string]()
			for k := range input.RemoveTags {
				if _, inAdd := input.AddTags[k]; !inAdd {
					tagEntry.Remove.Add(k)
				}
			}
		}
	}

	if err := u.Store.StageTag(service, name, tagEntry); err != nil {
		return nil, err
	}

	return &TagOutput{Name: name}, nil
}
