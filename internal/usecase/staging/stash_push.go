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
	// Keep preserves the agent memory after persisting.
	Keep bool
	// Mode determines how to handle existing stash file.
	// StashModeMerge combines agent data with existing file data.
	// StashModeOverwrite replaces existing file data.
	Mode StashMode
}

// StashPushOutput holds the result of the persist use case.
type StashPushOutput struct {
	// EntryCount is the number of entries persisted.
	EntryCount int
	// TagCount is the number of tag entries persisted.
	TagCount int
}

// StashPushUseCase executes persist operations (agent -> file).
type StashPushUseCase struct {
	AgentStore store.AgentStore
	FileStore  store.FileStore
}

// Execute runs the persist use case.
func (u *StashPushUseCase) Execute(ctx context.Context, input StashPushInput) (*StashPushOutput, error) {
	// Drain state from agent (keep for now, will clear after successful file write if needed)
	agentState, err := u.AgentStore.Drain(ctx, "", true)
	if err != nil {
		return nil, &StashPushError{Op: "load", Err: err}
	}

	// Extract service-specific state if filtered
	persistState := agentState.ExtractService(input.Service)

	// Check if there's anything to persist
	if persistState.IsEmpty() {
		return nil, ErrNothingToStashPush
	}

	// Determine final state based on strategy and scope
	var finalState *staging.State

	switch {
	case input.Service != "":
		// Service-specific: always preserve other services from file
		fileState, err := u.FileStore.Drain(ctx, "", true)
		if err != nil {
			// File might not exist, which is fine - start fresh
			fileState = staging.NewEmptyState()
		}

		finalState = fileState
		if input.Mode == StashModeOverwrite {
			// Overwrite: clear target service, then add agent's data
			finalState.RemoveService(input.Service)
			finalState.Merge(persistState)
		} else {
			// Merge: combine file's target service with agent's target service
			finalState.Merge(persistState)
		}
	case input.Mode == StashModeMerge:
		// Global merge: combine file state with agent state
		fileState, err := u.FileStore.Drain(ctx, "", true)
		if err != nil {
			// File might not exist, which is fine - start fresh
			fileState = staging.NewEmptyState()
		}

		finalState = fileState
		finalState.Merge(persistState)
	default:
		// Global overwrite: replace entire file with agent state
		finalState = persistState
	}

	// Write to file
	if err := u.FileStore.WriteState(ctx, "", finalState); err != nil {
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

	// Clear agent memory for the persisted service unless --keep is specified
	if !input.Keep {
		if input.Service != "" {
			// Remove only the persisted service from agent, keep the rest
			agentState.RemoveService(input.Service)

			if err := u.AgentStore.WriteState(ctx, "", agentState); err != nil {
				return output, &StashPushError{Op: "clear", Err: err, NonFatal: true}
			}
		} else {
			// Clear all memory with persist hint for proper shutdown message
			if hinted, ok := u.AgentStore.(store.HintedUnstager); ok {
				if err := hinted.UnstageAllWithHint(ctx, "", store.HintPersist); err != nil {
					return output, &StashPushError{Op: "clear", Err: err, NonFatal: true}
				}
			} else if err := u.AgentStore.UnstageAll(ctx, ""); err != nil {
				return output, &StashPushError{Op: "clear", Err: err, NonFatal: true}
			}
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
		return "failed to get state from agent: " + e.Err.Error()
	case "write":
		return "failed to save state to file: " + e.Err.Error()
	case "clear":
		return "failed to clear agent memory: " + e.Err.Error()
	default:
		return e.Err.Error()
	}
}

func (e *StashPushError) Unwrap() error {
	return e.Err
}
