package staging

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/transition"
)

// EditInput holds input for the edit use case.
type EditInput struct {
	Name        string
	Value       string
	Description string
}

// EditOutput holds the result of the edit use case.
type EditOutput struct {
	Name     string
	Skipped  bool // True if the edit was skipped because value matches AWS
	Unstaged bool // True if the entry was auto-unstaged
}

// EditUseCase executes edit operations.
type EditUseCase struct {
	Strategy staging.EditStrategy
	Store    staging.StoreReadWriter
}

// Execute runs the edit use case.
func (u *EditUseCase) Execute(ctx context.Context, input EditInput) (*EditOutput, error) {
	service := u.Strategy.Service()

	// Load current state from AWS
	currentValue, awsBaseModifiedAt, err := u.fetchCurrentState(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	// Load staged entry state with metadata
	entryState, existingBaseModifiedAt, err := transition.LoadEntryStateWithMetadata(u.Store, service, input.Name, currentValue)
	if err != nil {
		return nil, err
	}

	// Use existing BaseModifiedAt if available, otherwise use AWS
	baseModifiedAt := existingBaseModifiedAt
	if baseModifiedAt == nil {
		baseModifiedAt = awsBaseModifiedAt
	}

	// Build options with metadata
	opts := &transition.EntryExecuteOptions{
		BaseModifiedAt: baseModifiedAt,
	}
	if input.Description != "" {
		opts.Description = &input.Description
	}

	// Execute the transition
	executor := transition.NewExecutor(u.Store)
	_, wasNotStaged := entryState.StagedState.(transition.EntryStagedStateNotStaged)

	result, err := executor.ExecuteEntry(service, input.Name, entryState, transition.EntryActionEdit{Value: input.Value}, opts)
	if err != nil {
		return nil, err
	}

	// Check if skipped or unstaged
	output := &EditOutput{Name: input.Name}
	_, isNotStaged := result.NewState.StagedState.(transition.EntryStagedStateNotStaged)

	if wasNotStaged && isNotStaged {
		output.Skipped = true
	} else if !wasNotStaged && isNotStaged {
		output.Unstaged = true
	}

	return output, nil
}

// fetchCurrentState fetches the current AWS value and last modified time.
func (u *EditUseCase) fetchCurrentState(ctx context.Context, name string) (*string, *time.Time, error) {
	result, err := u.Strategy.FetchCurrentValue(ctx, name)
	if err != nil {
		return nil, nil, err
	}

	var currentValue *string
	if result.Value != "" {
		currentValue = &result.Value
	}

	var baseModifiedAt *time.Time
	if !result.LastModified.IsZero() {
		baseModifiedAt = &result.LastModified
	}

	return currentValue, baseModifiedAt, nil
}

// BaselineInput holds input for getting baseline value.
type BaselineInput struct {
	Name string
}

// BaselineOutput holds the baseline value for editing.
type BaselineOutput struct {
	Value        string
	IsStagedEdit bool // True if the baseline is from a staged edit (not AWS)
}

// Baseline returns the baseline value for editing (staged value if exists, otherwise from AWS).
func (u *EditUseCase) Baseline(ctx context.Context, input BaselineInput) (*BaselineOutput, error) {
	service := u.Strategy.Service()

	// Check if already staged
	stagedEntry, err := u.Store.GetEntry(service, input.Name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}

	if stagedEntry != nil {
		switch stagedEntry.Operation {
		case staging.OperationCreate, staging.OperationUpdate:
			return &BaselineOutput{
				Value:        lo.FromPtr(stagedEntry.Value),
				IsStagedEdit: true,
			}, nil
		case staging.OperationDelete:
			// BLOCKED: Cannot edit something staged for deletion
			return nil, transition.ErrCannotEditDelete
		}
	}

	// Fetch from AWS
	result, err := u.Strategy.FetchCurrentValue(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	return &BaselineOutput{Value: result.Value}, nil
}
