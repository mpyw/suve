package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/transition"
)

// DeleteInput holds input for the delete use case.
type DeleteInput struct {
	Name           string
	Force          bool // For Secrets Manager: force immediate deletion
	RecoveryWindow int  // For Secrets Manager: days before permanent deletion (7-30)
}

// DeleteOutput holds the result of the delete use case.
type DeleteOutput struct {
	Name              string
	Unstaged          bool // True if a staged CREATE was removed instead of staging DELETE
	ShowDeleteOptions bool // True if delete options (Force/RecoveryWindow) should be shown
	Force             bool
	RecoveryWindow    int
}

// DeleteUseCase executes delete staging operations.
type DeleteUseCase struct {
	Strategy staging.DeleteStrategy
	Store    staging.StoreReadWriteOperator
}

// Execute runs the delete use case.
func (u *DeleteUseCase) Execute(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	service := u.Strategy.Service()
	itemName := u.Strategy.ItemName()
	hasDeleteOptions := u.Strategy.HasDeleteOptions()

	// Validate recovery window if delete options are supported
	if hasDeleteOptions && !input.Force {
		if input.RecoveryWindow < 7 || input.RecoveryWindow > 30 {
			return nil, fmt.Errorf("recovery window must be between 7 and 30 days")
		}
	}

	// Fetch LastModified for conflict detection (needed for non-CREATE cases)
	lastModified, err := u.Strategy.FetchLastModified(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", itemName, err)
	}

	// Determine CurrentValue based on AWS existence
	var currentValue *string
	if !lastModified.IsZero() {
		// Resource exists on AWS - set a non-nil value to indicate existence
		// The actual value doesn't matter for delete, only existence check
		currentValue = new(string)
	}

	// Load current state with CurrentValue for existence check
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, input.Name, currentValue)
	if err != nil {
		return nil, err
	}

	// Use reducer to determine transition - existence check is done in reducer
	result := transition.ReduceEntry(entryState, transition.EntryActionDelete{})
	if result.Error != nil {
		return nil, result.Error
	}

	// Check if we should unstage a CREATE
	if result.DiscardTags {
		// CREATE -> NotStaged: unstage entry and tags
		if err := u.Store.UnstageEntry(ctx, service, input.Name); err != nil {
			return nil, err
		}
		// Unstage tags too (ignore ErrNotStaged)
		if err := u.Store.UnstageTag(ctx, service, input.Name); err != nil && !errors.Is(err, staging.ErrNotStaged) {
			return nil, err
		}
		return &DeleteOutput{
			Name:     input.Name,
			Unstaged: true,
		}, nil
	}

	// Stage delete with options (single persist)
	if err := u.stageDeleteWithOptions(ctx, service, input.Name, lastModified, hasDeleteOptions, input.Force, input.RecoveryWindow); err != nil {
		return nil, err
	}

	output := &DeleteOutput{
		Name:              input.Name,
		ShowDeleteOptions: hasDeleteOptions,
	}
	if hasDeleteOptions {
		output.Force = input.Force
		output.RecoveryWindow = input.RecoveryWindow
	}

	return output, nil
}

// stageDeleteWithOptions stages a delete entry with optional delete options.
func (u *DeleteUseCase) stageDeleteWithOptions(ctx context.Context, service staging.Service, name string, lastModified time.Time, hasDeleteOptions, force bool, recoveryWindow int) error {
	entry := staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}
	if !lastModified.IsZero() {
		entry.BaseModifiedAt = &lastModified
	}

	if hasDeleteOptions {
		entry.DeleteOptions = &staging.DeleteOptions{
			Force:          force,
			RecoveryWindow: recoveryWindow,
		}
	}

	return u.Store.StageEntry(ctx, service, name, entry)
}
