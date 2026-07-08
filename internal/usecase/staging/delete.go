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

// DeleteInput holds input for the delete use case.
type DeleteInput struct {
	Name           string
	Force          bool // For Secrets Manager: force immediate deletion
	RecoveryWindow int  // For Secrets Manager: days before permanent deletion (7-30)
	// Namespace is the Azure App Configuration namespace of the setting being
	// deleted; empty is the null/default namespace and the only value for every
	// other provider.
	Namespace string
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

	fetched, err := u.Strategy.FetchLastModified(ctx, input.Name)
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

	// Load current state with CurrentValue for existence check
	entryState, err := transition.LoadEntryState(ctx, u.Store, service, input.Name, input.Namespace, currentValue)
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
		if err := u.Store.UnstageEntry(ctx, service, input.Name, input.Namespace); err != nil {
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
	if err := u.stageDeleteWithOptions(
		ctx, service, input.Name, input.Namespace, lastModified, hasDeleteOptions, input.Force, input.RecoveryWindow,
	); err != nil {
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
//
//nolint:lll // function parameters are descriptive for clarity
func (u *DeleteUseCase) stageDeleteWithOptions(ctx context.Context, service staging.Service, name, namespace string, lastModified time.Time, hasDeleteOptions, force bool, recoveryWindow int) error {
	entry := staging.Entry{
		Operation: staging.OperationDelete,
		Namespace: namespace,
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
