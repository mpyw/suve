package staging

import (
	"context"
	"fmt"
	"time"

	"github.com/mpyw/suve/internal/staging"
)

// DeleteStrategy provides delete-specific operations.
type DeleteStrategy interface {
	staging.ServiceStrategy
	FetchLastModified(ctx context.Context, name string) (time.Time, error)
}

// DeleteInput holds input for the delete use case.
type DeleteInput struct {
	Name           string
	Force          bool // For Secrets Manager: force immediate deletion
	RecoveryWindow int  // For Secrets Manager: days before permanent deletion (7-30)
}

// DeleteOutput holds the result of the delete use case.
type DeleteOutput struct {
	Name              string
	HasDeleteOptions  bool
	Force             bool
	RecoveryWindow    int
	ShowDeleteOptions bool
}

// DeleteUseCase executes delete staging operations.
type DeleteUseCase struct {
	Strategy DeleteStrategy
	Store    *staging.Store
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
