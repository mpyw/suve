package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// DrainInput holds input for the drain use case.
type DrainInput struct {
	// Service filters the drain to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the file after draining.
	Keep bool
	// Force overwrites agent memory without checking for conflicts.
	Force bool
	// Merge combines file changes with existing agent memory.
	Merge bool
}

// DrainOutput holds the result of the drain use case.
type DrainOutput struct {
	// Merged indicates whether the state was merged with existing agent state.
	Merged bool
	// EntryCount is the number of entries in the final state.
	EntryCount int
	// TagCount is the number of tag entries in the final state.
	TagCount int
}

// DrainUseCase executes drain operations (file -> agent).
type DrainUseCase struct {
	FileStore  store.FileStore
	AgentStore store.AgentStore
}

// Execute runs the drain use case.
func (u *DrainUseCase) Execute(ctx context.Context, input DrainInput) (*DrainOutput, error) {
	// Drain from file (keep file for now, we'll delete after successful agent write)
	fileState, err := u.FileStore.Drain(ctx, true)
	if err != nil {
		return nil, &DrainError{Op: "load", Err: err}
	}

	// Extract service-specific state if filtered
	drainState := fileState.ExtractService(input.Service)

	// Check if there's anything to drain
	if drainState.IsEmpty() {
		return nil, ErrNothingToDrain
	}

	// Check if agent already has staged changes
	agentState, err := u.AgentStore.Drain(ctx, true) // keep=true to not clear yet
	if err != nil {
		// Agent might not be running, which is fine - treat as empty
		agentState = staging.NewEmptyState()
	}

	// Check for conflicts
	agentServiceState := agentState.ExtractService(input.Service)
	if !agentServiceState.IsEmpty() && !input.Force && !input.Merge {
		return nil, ErrAgentHasChanges
	}

	// Prepare final state
	var finalState *staging.State
	merged := false
	if input.Merge && !agentState.IsEmpty() {
		// Merge states: start with agent state, merge drain state (drain takes precedence)
		finalState = agentState
		finalState.Merge(drainState)
		merged = true
	} else if input.Service != "" {
		// Service-specific: merge with existing agent state for other services
		finalState = agentState
		finalState.RemoveService(input.Service) // Clear the target service
		finalState.Merge(drainState)
	} else {
		// Replace all: use file state directly
		finalState = drainState
	}

	// Set state in agent
	if err := u.AgentStore.WriteState(ctx, finalState); err != nil {
		return nil, &DrainError{Op: "write", Err: err}
	}

	// Prepare output (before cleanup, so we can return it even on non-fatal errors)
	entryCount := 0
	for _, entries := range finalState.Entries {
		entryCount += len(entries)
	}
	tagCount := 0
	for _, tags := range finalState.Tags {
		tagCount += len(tags)
	}
	output := &DrainOutput{
		Merged:     merged,
		EntryCount: entryCount,
		TagCount:   tagCount,
	}

	// Delete file content (service-specific or all)
	if !input.Keep {
		if input.Service != "" {
			// Remove only the drained service from file, keep the rest
			fileState.RemoveService(input.Service)
			if fileState.IsEmpty() {
				// Delete the file entirely
				if _, err := u.FileStore.Drain(ctx, false); err != nil {
					// Non-fatal: state is already in agent
					return output, &DrainError{Op: "delete", Err: err, NonFatal: true}
				}
			} else {
				// Write back the remaining state
				if err := u.FileStore.WriteState(ctx, fileState); err != nil {
					return output, &DrainError{Op: "delete", Err: err, NonFatal: true}
				}
			}
		} else {
			// Drain again with keep=false to delete the file
			if _, err := u.FileStore.Drain(ctx, false); err != nil {
				return output, &DrainError{Op: "delete", Err: err, NonFatal: true}
			}
		}
	}

	return output, nil
}

var (
	// ErrNothingToDrain is returned when there are no staged changes in file to drain.
	ErrNothingToDrain = errors.New("no staged changes in file to drain")
	// ErrAgentHasChanges is returned when agent has staged changes and neither force nor merge is specified.
	ErrAgentHasChanges = errors.New("agent already has staged changes; use --force to overwrite or --merge to combine")
)

// DrainError represents an error during drain operation.
type DrainError struct {
	Op       string // "load", "write", "delete"
	Err      error
	NonFatal bool // If true, the error is non-fatal (state was already written)
}

func (e *DrainError) Error() string {
	switch e.Op {
	case "load":
		return "failed to load state from file: " + e.Err.Error()
	case "write":
		return "failed to set state in agent: " + e.Err.Error()
	case "delete":
		return "failed to delete file: " + e.Err.Error()
	default:
		return e.Err.Error()
	}
}

func (e *DrainError) Unwrap() error {
	return e.Err
}
