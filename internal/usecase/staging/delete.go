package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mpyw/suve/internal/staging"
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
	HasDeleteOptions  bool
	Force             bool
	RecoveryWindow    int
	ShowDeleteOptions bool
}

// DeleteUseCase executes delete staging operations.
type DeleteUseCase struct {
	Strategy staging.DeleteStrategy
	Store    staging.StoreReadWriter
}

// Execute runs the delete use case.
func (u *DeleteUseCase) Execute(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	service := u.Strategy.Service()
	itemName := u.Strategy.ItemName()
	hasDeleteOptions := u.Strategy.HasDeleteOptions()

	// Check if CREATE is staged - if so, just unstage instead of staging DELETE
	existingEntry, err := u.Store.Get(service, input.Name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}
	if existingEntry != nil && existingEntry.Operation == staging.OperationCreate {
		// Unstage the CREATE instead of staging DELETE
		if err := u.Store.Unstage(service, input.Name); err != nil {
			return nil, err
		}
		return &DeleteOutput{
			Name:     input.Name,
			Unstaged: true,
		}, nil
	}

	// Validate recovery window if delete options are supported
	if hasDeleteOptions && !input.Force {
		if input.RecoveryWindow < 7 || input.RecoveryWindow > 30 {
			return nil, fmt.Errorf("recovery window must be between 7 and 30 days")
		}
	}

	// Fetch LastModified for conflict detection
	lastModified, err := u.Strategy.FetchLastModified(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", itemName, err)
	}

	entry := staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}
	if !lastModified.IsZero() {
		entry.BaseModifiedAt = &lastModified
	}

	if hasDeleteOptions {
		entry.DeleteOptions = &staging.DeleteOptions{
			Force:          input.Force,
			RecoveryWindow: input.RecoveryWindow,
		}
	}

	if err := u.Store.Stage(service, input.Name, entry); err != nil {
		return nil, err
	}

	output := &DeleteOutput{
		Name:              input.Name,
		HasDeleteOptions:  hasDeleteOptions,
		ShowDeleteOptions: hasDeleteOptions,
	}
	if hasDeleteOptions && entry.DeleteOptions != nil {
		output.Force = entry.DeleteOptions.Force
		output.RecoveryWindow = entry.DeleteOptions.RecoveryWindow
	}

	return output, nil
}
