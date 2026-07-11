package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/transition"
)

// DeleteInput holds input for the delete use case. Key identifies the item by
// name and (Azure App Configuration) namespace; the namespace is empty for the
// null/default namespace and every other provider.
type DeleteInput struct {
	Key            staging.EntryKey
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
	Store    store.ReadWriteOperator
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

	// Fetch LastModified to determine existence (and, for existing resources, to
	// record a conflict-detection base). Existence is inferred from the ERROR,
	// not from the returned timestamp: a resource that exists but carries no
	// modification time still yields a zero time and must NOT be misread as
	// "not found".
	var (
		lastModified time.Time
		currentValue *string
	)

	fetched, err := u.Strategy.FetchLastModified(ctx, input.Key.Name)
	if err != nil {
		// ResourceNotFoundError means the resource doesn't exist; leave
		// currentValue nil so the reducer either errors (nothing to delete) or
		// unstages a staged CREATE. Any other error is a genuine failure.
		if notFoundErr := (*staging.ResourceNotFoundError)(nil); !errors.As(err, &notFoundErr) {
			return nil, fmt.Errorf("failed to fetch %s: %w", itemName, err)
		}
	} else {
		// Resource exists on AWS - a non-nil currentValue signals existence to
		// the reducer. A zero fetched time here means "exists, modification time
		// unknown" and is preserved as-is.
		lastModified = fetched
		currentValue = new(string)
	}

	key := input.Key

	// Load current state with CurrentValue for existence check
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, key, currentValue)
	if err != nil {
		return nil, err
	}

	// Use reducer to determine transition - existence check is done in reducer
	result := transition.ReduceEntry(entryState, transition.EntryActionDelete{})
	if result.Error != nil {
		return nil, result.Error
	}

	// A CREATE reduces to NotStaged (nothing to delete on AWS); anything else
	// stages a DELETE. In both cases DiscardTags may be set, and the resource
	// ends up gone, so orphan staged tags must be unstaged too.
	_, unstaged := result.NewState.StagedState.(transition.EntryStagedStateNotStaged)

	if unstaged {
		// CREATE -> NotStaged: unstage the entry.
		if err := u.Store.UnstageEntry(ctx, service, key); err != nil {
			return nil, err
		}
	} else {
		// Stage delete with options (single persist)
		if err := u.stageDeleteWithOptions(
			ctx, service, key, lastModified, hasDeleteOptions, input.Force, input.RecoveryWindow,
		); err != nil {
			return nil, err
		}
	}

	// Discard orphan staged tags (ignore ErrNotStaged) - the resource is gone.
	if result.DiscardTags {
		if err := u.Store.UnstageTag(ctx, service, key); err != nil && !errors.Is(err, staging.ErrNotStaged) {
			return nil, err
		}
	}

	if unstaged {
		return &DeleteOutput{
			Name:     input.Key.Name,
			Unstaged: true,
		}, nil
	}

	output := &DeleteOutput{
		Name:              input.Key.Name,
		ShowDeleteOptions: hasDeleteOptions,
	}
	if hasDeleteOptions {
		output.Force = input.Force
		output.RecoveryWindow = input.RecoveryWindow
	}

	return output, nil
}

// stageDeleteWithOptions stages a delete entry with optional delete options.
//
//nolint:lll // function parameters are descriptive for clarity
func (u *DeleteUseCase) stageDeleteWithOptions(ctx context.Context, service staging.Service, key staging.EntryKey, lastModified time.Time, hasDeleteOptions, force bool, recoveryWindow int) error {
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

	return u.Store.StageEntry(ctx, service, key, entry)
}
