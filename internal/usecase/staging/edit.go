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
	Tags        map[string]string
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

	// Get base modified time
	baseModifiedAt, err := u.getBaseModifiedAt(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	// Stage the change
	entry := staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr(input.Value),
		StagedAt:       time.Now(),
		BaseModifiedAt: baseModifiedAt,
	}
	if input.Description != "" {
		entry.Description = &input.Description
	}
	if len(input.Tags) > 0 {
		entry.Tags = input.Tags
	}
	if err := u.Store.Stage(service, input.Name, entry); err != nil {
		return nil, err
	}

	return &EditOutput{Name: input.Name}, nil
}

func (u *EditUseCase) getBaseModifiedAt(ctx context.Context, name string) (*time.Time, error) {
	service := u.Strategy.Service()

	// Check if already staged
	stagedEntry, err := u.Store.Get(service, name)
	if err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}

	if stagedEntry != nil && (stagedEntry.Operation == staging.OperationCreate || stagedEntry.Operation == staging.OperationUpdate) {
		return stagedEntry.BaseModifiedAt, nil
	}

	// Fetch from AWS
	result, err := u.Strategy.FetchCurrentValue(ctx, name)
	if err != nil {
		return nil, err
	}
	if !result.LastModified.IsZero() {
		return &result.LastModified, nil
	}
	return nil, nil
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

	if stagedEntry != nil && (stagedEntry.Operation == staging.OperationCreate || stagedEntry.Operation == staging.OperationUpdate) {
		return &BaselineOutput{
			Value: lo.FromPtr(stagedEntry.Value),
		}, nil
	}

	// Fetch from AWS
	result, err := u.Strategy.FetchCurrentValue(ctx, input.Name)
	if err != nil {
		return nil, err
	}

	return &BaselineOutput{Value: result.Value}, nil
}
