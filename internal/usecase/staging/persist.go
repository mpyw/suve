package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// PersistMode determines how to handle existing stash file.
type PersistMode int

const (
	// PersistModeOverwrite replaces the existing stash file.
	PersistModeOverwrite PersistMode = iota
	// PersistModeMerge merges with the existing stash file.
	PersistModeMerge
)

// PersistInput holds input for the persist use case.
type PersistInput struct {
	// Service filters the persist to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the agent memory after persisting.
	Keep bool
	// Mode determines how to handle existing stash file.
	Mode PersistMode
}

// PersistOutput holds the result of the persist use case.
type PersistOutput struct {
	// EntryCount is the number of entries persisted.
	EntryCount int
	// TagCount is the number of tag entries persisted.
	TagCount int
}

// PersistUseCase executes persist operations (agent -> file).
type PersistUseCase struct {
	AgentStore store.AgentStore
	FileStore  store.FileStore
}

// Execute runs the persist use case.
func (u *PersistUseCase) Execute(ctx context.Context, input PersistInput) (*PersistOutput, error) {
	// Drain state from agent (keep for now, will clear after successful file write if needed)
	agentState, err := u.AgentStore.Drain(ctx, "", true)
	if err != nil {
		return nil, &PersistError{Op: "load", Err: err}
	}

	// Extract service-specific state if filtered
	persistState := agentState.ExtractService(input.Service)

	// Check if there's anything to persist
	if persistState.IsEmpty() {
		return nil, ErrNothingToPersist
	}

	// Load existing file state to merge with (for merge mode or service-specific persist)
	var finalState *staging.State
	if input.Mode == PersistModeMerge || input.Service != "" {
		// Merge mode or service-specific: merge with existing file state
		fileState, err := u.FileStore.Drain(ctx, "", true)
		if err != nil {
			// File might not exist, which is fine - start fresh
			fileState = staging.NewEmptyState()
		}
		finalState = fileState
		if input.Service != "" {
			finalState.RemoveService(input.Service) // Clear the target service only
		}
		finalState.Merge(persistState)
	} else {
		// Replace all: use agent state directly
		finalState = persistState
	}

	// Write to file
	if err := u.FileStore.WriteState(ctx, "", finalState); err != nil {
		return nil, &PersistError{Op: "write", Err: err}
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
	output := &PersistOutput{
		EntryCount: entryCount,
		TagCount:   tagCount,
	}

	// Clear agent memory for the persisted service unless --keep is specified
	if !input.Keep {
		if input.Service != "" {
			// Remove only the persisted service from agent, keep the rest
			agentState.RemoveService(input.Service)
			if err := u.AgentStore.WriteState(ctx, "", agentState); err != nil {
				return output, &PersistError{Op: "clear", Err: err, NonFatal: true}
			}
		} else {
			// Clear all memory with persist hint for proper shutdown message
			if hinted, ok := u.AgentStore.(store.HintedUnstager); ok {
				if err := hinted.UnstageAllWithHint(ctx, "", store.HintPersist); err != nil {
					return output, &PersistError{Op: "clear", Err: err, NonFatal: true}
				}
			} else if err := u.AgentStore.UnstageAll(ctx, ""); err != nil {
				return output, &PersistError{Op: "clear", Err: err, NonFatal: true}
			}
		}
	}

	return output, nil
}

var (
	// ErrNothingToPersist is returned when there are no staged changes to persist.
	ErrNothingToPersist = errors.New("no staged changes to persist")
)

// PersistError represents an error during persist operation.
type PersistError struct {
	Op       string // "load", "write", "clear"
	Err      error
	NonFatal bool // If true, the error is non-fatal (state was already written)
}

func (e *PersistError) Error() string {
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

func (e *PersistError) Unwrap() error {
	return e.Err
}
