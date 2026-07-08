package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// Stash error Op codes shared by StashPopError and StashPushError.
const (
	stashOpLoad        = "load"
	stashOpWrite       = "write"
	stashOpDelete      = "delete"
	stashOpReadStash   = "read-stash"
	stashOpReadWorking = "read-working"
)

// StashPopInput holds input for the drain use case.
type StashPopInput struct {
	// Service filters the drain to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the stash file after popping.
	Keep bool
	// Mode determines how to handle conflicts with existing working staging area.
	// StashModeMerge combines stash changes with the existing working staging area.
	// StashModeOverwrite replaces the existing working staging area.
	Mode StashMode
}

// StashPopOutput holds the result of the drain use case.
type StashPopOutput struct {
	// Merged indicates whether the state was merged with the existing working state.
	Merged bool
	// EntryCount is the number of entries in the final state.
	EntryCount int
	// TagCount is the number of tag entries in the final state.
	TagCount int
}

// StashPopUseCase executes drain operations (stash.json -> working param.json/secret.json).
type StashPopUseCase struct {
	// Stash is the stash file (stash.json).
	Stash store.FileStore
	// Working is the working staging area (param.json/secret.json).
	Working store.FileStore
}

// Execute runs the drain use case.
func (u *StashPopUseCase) Execute(ctx context.Context, input StashPopInput) (*StashPopOutput, error) {
	// Read from the stash (keep for now, we'll delete after successful working write)
	stashState, err := u.Stash.Drain(ctx, "", true)
	if err != nil {
		return nil, &StashPopError{Op: stashOpLoad, Err: err}
	}

	// Extract service-specific state if filtered
	drainState := stashState.ExtractService(input.Service)

	// Check if there's anything to drain
	if drainState.IsEmpty() {
		return nil, ErrNothingToStashPop
	}

	// Read the working staging area (keep=true to not clear yet). A missing file
	// yields an empty state with a nil error, so any error here is a real
	// failure (wrong key, corrupt/unreadable file): propagate it BEFORE the
	// WriteState below would replace working files — and deleting any service
	// whose slice ended up empty — with a partial view.
	workingState, err := u.Working.Drain(ctx, "", true)
	if err != nil {
		return nil, &StashPopError{Op: stashOpReadWorking, Err: err}
	}

	// Determine final state based on mode and scope
	var finalState *staging.State

	merged := false

	// Check if working area has data BEFORE any modifications (for merged output)
	workingServiceState := workingState.ExtractService(input.Service)
	hasExistingData := !workingServiceState.IsEmpty()
	workingWasEmpty := workingState.IsEmpty()

	switch {
	case input.Service != "":
		// Service-specific: always preserve other services from working area
		finalState = workingState
		if input.Mode == StashModeOverwrite {
			// Overwrite: clear target service, then add stash's data
			finalState.RemoveService(input.Service)
			finalState.Merge(drainState)
		} else {
			// Merge: combine working's target service with stash's target service
			finalState.Merge(drainState)

			merged = hasExistingData
		}
	case input.Mode == StashModeMerge:
		// Global merge: combine working state with stash state
		finalState = workingState
		finalState.Merge(drainState)

		merged = !workingWasEmpty
	default:
		// Global overwrite: replace entire working state with stash state
		finalState = drainState
	}

	// Set state in the working staging area
	if err := u.Working.WriteState(ctx, "", finalState); err != nil {
		return nil, &StashPopError{Op: stashOpWrite, Err: err}
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

	// Delete stash content (service-specific or all)
	if !input.Keep {
		if input.Service != "" {
			// Remove only the drained service from stash, keep the rest
			stashState.RemoveService(input.Service)

			if stashState.IsEmpty() {
				// Delete the stash file entirely
				if _, err := u.Stash.Drain(ctx, "", false); err != nil {
					// Non-fatal: state is already in the working area
					return output, &StashPopError{Op: stashOpDelete, Err: err, NonFatal: true}
				}
			} else {
				// Write back the remaining state
				if err := u.Stash.WriteState(ctx, "", stashState); err != nil {
					return output, &StashPopError{Op: stashOpDelete, Err: err, NonFatal: true}
				}
			}
		} else {
			// Drain again with keep=false to delete the stash file
			if _, err := u.Stash.Drain(ctx, "", false); err != nil {
				return output, &StashPopError{Op: stashOpDelete, Err: err, NonFatal: true}
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
	case stashOpLoad:
		return "failed to load state from file: " + e.Err.Error()
	case stashOpWrite:
		return "failed to write the working staging area: " + e.Err.Error()
	case stashOpDelete:
		return "failed to delete file: " + e.Err.Error()
	case stashOpReadWorking:
		return "failed to read the working staging area: " + e.Err.Error()
	default:
		return e.Err.Error()
	}
}

func (e *StashPopError) Unwrap() error {
	return e.Err
}
