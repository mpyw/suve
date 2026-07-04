package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// StashPushInput holds input for the persist use case.
type StashPushInput struct {
	// Service filters the persist to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the working staging area after persisting.
	Keep bool
	// Mode determines how to handle existing stash file.
	// StashModeMerge combines working data with existing stash data.
	// StashModeOverwrite replaces existing stash data.
	Mode StashMode
}

// StashPushOutput holds the result of the persist use case.
type StashPushOutput struct {
	// EntryCount is the number of entries persisted.
	EntryCount int
	// TagCount is the number of tag entries persisted.
	TagCount int
}

// StashPushUseCase executes persist operations (working stage.json -> stash.json).
type StashPushUseCase struct {
	// Working is the working staging area (stage.json).
	Working store.FileStore
	// Stash is the stash file (stash.json).
	Stash store.FileStore
}

// Execute runs the persist use case.
func (u *StashPushUseCase) Execute(ctx context.Context, input StashPushInput) (*StashPushOutput, error) {
	// Read state from the working staging area (keep for now, will clear after successful stash write if needed)
	workingState, err := u.Working.Drain(ctx, "", true)
	if err != nil {
		return nil, &StashPushError{Op: "load", Err: err}
	}

	// Extract service-specific state if filtered
	persistState := workingState.ExtractService(input.Service)

	// Check if there's anything to persist
	if persistState.IsEmpty() {
		return nil, ErrNothingToStashPush
	}

	// Determine final state based on strategy and scope
	var finalState *staging.State

	switch {
	case input.Service != "":
		// Service-specific: always preserve other services from stash
		stashState, err := u.Stash.Drain(ctx, "", true)
		if err != nil {
			// Stash might not exist, which is fine - start fresh
			stashState = staging.NewEmptyState()
		}

		finalState = stashState
		if input.Mode == StashModeOverwrite {
			// Overwrite: clear target service, then add working's data
			finalState.RemoveService(input.Service)
			finalState.Merge(persistState)
		} else {
			// Merge: combine stash's target service with working's target service
			finalState.Merge(persistState)
		}
	case input.Mode == StashModeMerge:
		// Global merge: combine stash state with working state
		stashState, err := u.Stash.Drain(ctx, "", true)
		if err != nil {
			// Stash might not exist, which is fine - start fresh
			stashState = staging.NewEmptyState()
		}

		finalState = stashState
		finalState.Merge(persistState)
	default:
		// Global overwrite: replace entire stash with working state
		finalState = persistState
	}

	// Write to stash file
	if err := u.Stash.WriteState(ctx, "", finalState); err != nil {
		return nil, &StashPushError{Op: "write", Err: err}
	}

	// Prepare output (before cleanup, so we can return it even on non-fatal errors)
	entryCount := 0
	for _, entries := range persistState.Entries {
		entryCount += len(entries)
	}

	tagCount := 0
	for _, tags := range persistState.Tags {
		tagCount += len(tags)
	}

	output := &StashPushOutput{
		EntryCount: entryCount,
		TagCount:   tagCount,
	}

	// Clear the working staging area for the persisted service unless --keep is specified
	if !input.Keep {
		if input.Service != "" {
			// Remove only the persisted service from working, keep the rest
			workingState.RemoveService(input.Service)
		} else {
			// Clear all working state
			workingState = staging.NewEmptyState()
		}

		if err := u.Working.WriteState(ctx, "", workingState); err != nil {
			return output, &StashPushError{Op: "clear", Err: err, NonFatal: true}
		}
	}

	return output, nil
}

var (
	// ErrNothingToStashPush is returned when there are no staged changes to persist.
	ErrNothingToStashPush = errors.New("no staged changes to persist")
)

// StashPushError represents an error during persist operation.
type StashPushError struct {
	Op       string // "load", "write", "clear"
	Err      error
	NonFatal bool // If true, the error is non-fatal (state was already written)
}

func (e *StashPushError) Error() string {
	switch e.Op {
	case "load":
		return "failed to read the working staging area: " + e.Err.Error()
	case "write":
		return "failed to save state to file: " + e.Err.Error()
	case "clear":
		return "failed to clear the working staging area: " + e.Err.Error()
	default:
		return e.Err.Error()
	}
}

func (e *StashPushError) Unwrap() error {
	return e.Err
}
