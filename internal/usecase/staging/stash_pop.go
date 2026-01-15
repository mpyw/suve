package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// StashPopInput holds input for the drain use case.
type StashPopInput struct {
	// Service filters the drain to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the file after draining.
	Keep bool
	// Mode determines how to handle conflicts with existing agent memory.
	// StashModeMerge combines file changes with existing agent memory.
	// StashModeOverwrite replaces existing agent memory.
	Mode StashMode
}

// StashPopOutput holds the result of the drain use case.
type StashPopOutput struct {
	// Merged indicates whether the state was merged with existing agent state.
	Merged bool
	// EntryCount is the number of entries in the final state.
	EntryCount int
	// TagCount is the number of tag entries in the final state.
	TagCount int
}

// StashPopUseCase executes drain operations (file -> agent).
type StashPopUseCase struct {
	FileStore  store.FileStore
	AgentStore store.AgentStore
}

// Execute runs the drain use case.
func (u *StashPopUseCase) Execute(ctx context.Context, input StashPopInput) (*StashPopOutput, error) {
	// Drain from file (keep file for now, we'll delete after successful agent write)
	fileState, err := u.FileStore.Drain(ctx, "", true)
	if err != nil {
		return nil, &StashPopError{Op: "load", Err: err}
	}

	// Extract service-specific state if filtered
	drainState := fileState.ExtractService(input.Service)

	// Check if there's anything to drain
	if drainState.IsEmpty() {
		return nil, ErrNothingToStashPop
	}

	// Check if agent already has staged changes
	agentState, err := u.AgentStore.Drain(ctx, "", true) // keep=true to not clear yet
	if err != nil {
		// Agent might not be running, which is fine - treat as empty
		agentState = staging.NewEmptyState()
	}

	// Determine final state based on mode and scope
	var finalState *staging.State

	merged := false

	// Check if agent has data BEFORE any modifications (for merged output)
	agentServiceState := agentState.ExtractService(input.Service)
	hasExistingData := !agentServiceState.IsEmpty()
	agentWasEmpty := agentState.IsEmpty()

	switch {
	case input.Service != "":
		// Service-specific: always preserve other services from agent
		finalState = agentState
		if input.Mode == StashModeOverwrite {
			// Overwrite: clear target service, then add file's data
			finalState.RemoveService(input.Service)
			finalState.Merge(drainState)
		} else {
			// Merge: combine agent's target service with file's target service
			finalState.Merge(drainState)

			merged = hasExistingData
		}
	case input.Mode == StashModeMerge:
		// Global merge: combine agent state with file state
		finalState = agentState
		finalState.Merge(drainState)

		merged = !agentWasEmpty
	default:
		// Global overwrite: replace entire agent state with file state
		finalState = drainState
	}

	// Set state in agent
	if err := u.AgentStore.WriteState(ctx, "", finalState); err != nil {
		return nil, &StashPopError{Op: "write", Err: err}
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

	output := &StashPopOutput{
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
				if _, err := u.FileStore.Drain(ctx, "", false); err != nil {
					// Non-fatal: state is already in agent
					return output, &StashPopError{Op: "delete", Err: err, NonFatal: true}
				}
			} else {
				// Write back the remaining state
				if err := u.FileStore.WriteState(ctx, "", fileState); err != nil {
					return output, &StashPopError{Op: "delete", Err: err, NonFatal: true}
				}
			}
		} else {
			// Drain again with keep=false to delete the file
			if _, err := u.FileStore.Drain(ctx, "", false); err != nil {
				return output, &StashPopError{Op: "delete", Err: err, NonFatal: true}
			}
		}
	}

	return output, nil
}

var (
	// ErrNothingToStashPop is returned when there are no staged changes in file to drain.
	ErrNothingToStashPop = errors.New("no staged changes in file to drain")
)

// StashPopError represents an error during drain operation.
type StashPopError struct {
	Op       string // "load", "write", "delete"
	Err      error
	NonFatal bool // If true, the error is non-fatal (state was already written)
}

func (e *StashPopError) Error() string {
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

func (e *StashPopError) Unwrap() error {
	return e.Err
}
