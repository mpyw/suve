package staging

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
)

// EditInput holds input for the edit use case.
type EditInput struct {
	Name        string
	Value       string
	Description string
}

// EditOutput holds the result of the edit use case.
type EditOutput struct {
	Name string
}

// EditUseCase executes edit operations.
type EditUseCase struct {
	Strategy staging.EditStrategy
	Store    staging.StoreReadWriter
}

// Execute runs the edit use case.
func (u *EditUseCase) Execute(ctx context.Context, input EditInput) (*EditOutput, error) {
	service := u.Strategy.Service()

	// Get existing staged entry to preserve operation type
	existingOp, baseModifiedAt, err := u.getExistingState(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	// Determine operation: preserve existing staged operation, default to Update
	operation := staging.OperationUpdate
	if existingOp != nil {
		operation = *existingOp
	}

	// Stage the change
	entry := staging.Entry{
		Operation:      operation,
		Value:          lo.ToPtr(input.Value),
		StagedAt:       time.Now(),
		BaseModifiedAt: baseModifiedAt,
	}
	if input.Description != "" {
		entry.Description = &input.Description
	}
	if err := u.Store.Stage(service, input.Name, entry); err != nil {
		return nil, err
	}

	return &EditOutput{Name: input.Name}, nil
}

// getExistingState returns the existing staged operation (if any) and base modified time.
func (u *EditUseCase) getExistingState(ctx context.Context, name string) (*staging.Operation, *time.Time, error) {
	service := u.Strategy.Service()

	// Check if already staged
	stagedEntry, err := u.Store.Get(service, name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, nil, err
	}

	if stagedEntry != nil {
		switch stagedEntry.Operation {
		case staging.OperationCreate, staging.OperationUpdate:
			// Preserve existing operation type
			return &stagedEntry.Operation, stagedEntry.BaseModifiedAt, nil
		case staging.OperationDelete:
			// Cancel deletion and convert to UPDATE - fall through to fetch from AWS
		}
	}

	// Not staged or staged for deletion - fetch base time from AWS for Update operation
	result, err := u.Strategy.FetchCurrentValue(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	if !result.LastModified.IsZero() {
		return nil, &result.LastModified, nil
	}
	return nil, nil, nil
}

// BaselineInput holds input for getting baseline value.
type BaselineInput struct {
	Name string
}

// BaselineOutput holds the baseline value for editing.
type BaselineOutput struct {
	Value string
}

// Baseline returns the baseline value for editing (staged value if exists, otherwise from AWS).
func (u *EditUseCase) Baseline(ctx context.Context, input BaselineInput) (*BaselineOutput, error) {
	service := u.Strategy.Service()

	// Check if already staged
	stagedEntry, err := u.Store.Get(service, input.Name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}

	if stagedEntry != nil {
		switch stagedEntry.Operation {
		case staging.OperationCreate, staging.OperationUpdate:
			return &BaselineOutput{
				Value: lo.FromPtr(stagedEntry.Value),
			}, nil
		case staging.OperationDelete:
			// When editing a staged deletion, fetch from AWS to allow editing
			// Fall through to fetch from AWS
		}
	}

	// Fetch from AWS
	result, err := u.Strategy.FetchCurrentValue(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	return &BaselineOutput{Value: result.Value}, nil
}
