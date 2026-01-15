package staging

import (
	"context"
	"errors"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
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
	Store    store.ReadWriteOperator
}

// Execute runs the edit use case.
func (u *EditUseCase) Execute(ctx context.Context, input EditInput) (*EditOutput, error) {
	service := u.Strategy.Service()

	// Check staged state first to avoid unnecessary AWS fetch
	// Only ping-check if Store implements Pinger
	var stagedEntry *staging.Entry

	if pinger, ok := u.Store.(store.Pinger); ok {
		if pinger.Ping(ctx) != nil {
			// Daemon not running → nothing staged, will fetch from AWS below
			stagedEntry = nil
		} else {
			// Daemon running → check staged state
			entry, err := u.Store.GetEntry(ctx, service, input.Name)
			if err != nil && !errors.Is(err, staging.ErrNotStaged) {
				return nil, err
			}

			stagedEntry = entry
		}
	} else {
		// Not Pinger (e.g., FileStore) → check staged state directly
		entry, err := u.Store.GetEntry(ctx, service, input.Name)
		if err != nil && !errors.Is(err, staging.ErrNotStaged) {
			return nil, err
		}

		stagedEntry = entry
	}

	// Determine if we need to fetch from AWS
	var currentValue *string
	var awsBaseModifiedAt *time.Time

	if stagedEntry != nil && stagedEntry.Operation == staging.OperationCreate {
		// Staged as Create → resource doesn't exist in AWS, skip fetch
		currentValue = nil
		awsBaseModifiedAt = nil
	} else {
		// Not staged or staged as Update/Delete → fetch from AWS
		var err error

		currentValue, awsBaseModifiedAt, err = u.fetchCurrentState(ctx, input.Name)
		if err != nil {
			return nil, err
		}
	}

	// Build entry state from already-fetched data (avoid redundant GetEntry call)
	entryState, existingBaseModifiedAt := u.buildEntryState(stagedEntry, currentValue)

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

	result, err := executor.ExecuteEntry(ctx, service, input.Name, entryState, transition.EntryActionEdit{Value: input.Value}, opts)
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

// buildEntryState constructs EntryState from already-fetched staged entry and AWS value.
func (u *EditUseCase) buildEntryState(stagedEntry *staging.Entry, currentAWSValue *string) (transition.EntryState, *time.Time) {
	state := transition.EntryState{
		CurrentValue: currentAWSValue,
		StagedState:  transition.EntryStagedStateNotStaged{},
	}

	var baseModifiedAt *time.Time

	if stagedEntry != nil {
		baseModifiedAt = stagedEntry.BaseModifiedAt

		switch stagedEntry.Operation {
		case staging.OperationCreate:
			state.StagedState = transition.EntryStagedStateCreate{
				DraftValue: lo.FromPtr(stagedEntry.Value),
			}
		case staging.OperationUpdate:
			state.StagedState = transition.EntryStagedStateUpdate{
				DraftValue: lo.FromPtr(stagedEntry.Value),
			}
		case staging.OperationDelete:
			state.StagedState = transition.EntryStagedStateDelete{}
		}
	}

	return state, baseModifiedAt
}

// fetchCurrentState fetches the current AWS value and last modified time.
func (u *EditUseCase) fetchCurrentState(ctx context.Context, name string) (*string, *time.Time, error) {
	result, err := u.Strategy.FetchCurrentValue(ctx, name)
	if err != nil {
		return nil, nil, err
	}

	// Always use the value pointer - empty string is a valid AWS value
	currentValue := &result.Value

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

	// Only ping-check if Store implements Pinger
	// - Pinger + ping fails → daemon not running, skip staged check
	// - Pinger + ping succeeds → daemon running, check staged
	// - Not Pinger (e.g., FileStore) → proceed to GetEntry directly
	if pinger, ok := u.Store.(store.Pinger); ok {
		if pinger.Ping(ctx) != nil {
			// Daemon not running → skip staged check, go to AWS
			return u.fetchBaselineFromAWS(ctx, input.Name)
		}
	}

	// Check if already staged
	stagedEntry, err := u.Store.GetEntry(ctx, service, input.Name)
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

	// Not staged → fetch from AWS
	return u.fetchBaselineFromAWS(ctx, input.Name)
}

// fetchBaselineFromAWS fetches the baseline value from AWS.
func (u *EditUseCase) fetchBaselineFromAWS(ctx context.Context, name string) (*BaselineOutput, error) {
	result, err := u.Strategy.FetchCurrentValue(ctx, name)
	if err != nil {
		return nil, err
	}

	return &BaselineOutput{Value: result.Value}, nil
}
