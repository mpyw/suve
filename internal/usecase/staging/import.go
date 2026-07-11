package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// ImportOp identifies the stage of an import operation that failed.
type ImportOp string

// Import error Op codes.
const (
	ImportOpLoad        ImportOp = "load"
	ImportOpWrite       ImportOp = "write"
	ImportOpReadWorking ImportOp = "read-working"
)

// EnvelopeReader reads a single service's staged state from an import source
// (typically a per-service envelope file). Adapters bind the source path, scope
// validation, and passphrase; the use case only supplies the service. For a
// missing file in the directory/global case the adapter returns an empty state
// with a nil error (an absent service is skipped, not an error).
type EnvelopeReader interface {
	// ReadState returns the decoded state for svc.
	ReadState(ctx context.Context, svc staging.Service) (*staging.State, error)
}

// ImportInput holds input for the import use case.
type ImportInput struct {
	// Service filters the import to a specific service. Empty means all services.
	Service staging.Service
	// Mode determines how to reconcile with an existing working staging area.
	// ImportModeMerge combines the imported state with the working area.
	// ImportModeOverwrite replaces the working area with the imported state.
	Mode ImportMode
}

// ImportOutput holds the result of the import use case.
type ImportOutput struct {
	// Merged indicates whether the imported state was merged with pre-existing
	// working state.
	Merged bool
	// EntryCount is the number of entries in the final working state.
	EntryCount int
	// TagCount is the number of tag entries in the final working state.
	TagCount int
}

// ImportUseCase reads an export source into the working staging area. It keeps
// the merge/overwrite reconciliation of the former stash pop for the working
// area (a legitimate conflict), but is read-only on the source: nothing is
// consumed or deleted, so there is no Keep concept.
type ImportUseCase struct {
	// Source provides the imported per-service state.
	Source EnvelopeReader
	// Working is the working staging area (param.json/secret.json).
	Working store.WorkingStore
}

// Execute runs the import use case.
func (u *ImportUseCase) Execute(ctx context.Context, input ImportInput) (*ImportOutput, error) {
	// Read the imported state from the source (read-only; never mutated).
	sourceState, err := u.readSource(ctx, input.Service)
	if err != nil {
		return nil, &ImportError{Op: ImportOpLoad, Err: err}
	}

	if sourceState.IsEmpty() {
		return nil, ErrNothingToImport
	}

	output := &ImportOutput{}

	// Reconcile the source into the working area as a single atomic
	// read-modify-write. Update re-reads the working state fresh under the store
	// lock and writes it back without releasing the lock, so a concurrent
	// StageEntry can no longer land between a stale snapshot and the write-back
	// and be silently clobbered. A missing working file yields an empty state
	// with a nil error, so any error is a real failure (wrong key, corrupt file).
	reconciled := false

	err = u.Working.Update(ctx, "", func(workingState *staging.State) error {
		reconcileImport(workingState, sourceState, input, output)

		reconciled = true

		return nil
	})
	if err != nil {
		// fn (reconcileImport) runs only after a successful read, so if it never
		// ran the read failed; otherwise the write-back did.
		if !reconciled {
			return nil, &ImportError{Op: ImportOpReadWorking, Err: err}
		}

		return nil, &ImportError{Op: ImportOpWrite, Err: err}
	}

	return output, nil
}

// reconcileImport merges sourceState into workingState in place per the import
// mode, then records the resulting counts and merge flag in output. It is pure
// (in-memory) so it can run inside the store's atomic Update callback.
func reconcileImport(workingState, sourceState *staging.State, input ImportInput, output *ImportOutput) {
	// Capture whether the working area already held data BEFORE any mutation, for
	// the Merged output flag.
	hasExistingData := !workingState.ExtractService(input.Service).IsEmpty()
	workingWasEmpty := workingState.IsEmpty()

	merged := false

	switch {
	case input.Service != "":
		// Service-specific: always preserve other services from the working area.
		if input.Mode == ImportModeOverwrite {
			workingState.RemoveService(input.Service)
			workingState.Merge(sourceState)
		} else {
			workingState.Merge(sourceState)

			merged = hasExistingData
		}
	case input.Mode == ImportModeMerge:
		// Global merge: combine working state with imported state.
		workingState.Merge(sourceState)

		merged = !workingWasEmpty
	default:
		// Global overwrite: replace only the services actually present in the
		// source. A partial import (e.g. a directory holding only param.json,
		// with secret.json skipped) must not wipe an untouched working service,
		// so services absent from the source are left intact.
		for _, svc := range []staging.Service{staging.ServiceParam, staging.ServiceSecret} {
			if !sourceState.ExtractService(svc).IsEmpty() {
				workingState.RemoveService(svc)
			}
		}

		workingState.Merge(sourceState)
	}

	output.Merged = merged
	output.EntryCount = workingState.EntryCount()
	output.TagCount = workingState.TagCount()
}

// readSource reads the imported state. With a service filter it reads that one
// service; otherwise it reads each service (param, secret) and merges them
// (a missing per-service file yields an empty state, so it is simply skipped).
func (u *ImportUseCase) readSource(ctx context.Context, service staging.Service) (*staging.State, error) {
	if service != "" {
		return u.Source.ReadState(ctx, service)
	}

	result := staging.NewEmptyState()

	for _, svc := range []staging.Service{staging.ServiceParam, staging.ServiceSecret} {
		state, err := u.Source.ReadState(ctx, svc)
		if err != nil {
			return nil, err
		}

		result.Merge(state)
	}

	return result, nil
}

var (
	// ErrNothingToImport is returned when the source holds no staged changes.
	ErrNothingToImport = errors.New("no staged changes to import")
)

// ImportError represents an error during an import operation.
type ImportError struct {
	Op  ImportOp
	Err error
}

func (e *ImportError) Error() string {
	switch e.Op {
	case ImportOpLoad:
		return "failed to read export file: " + e.Err.Error()
	case ImportOpWrite:
		return "failed to write the working staging area: " + e.Err.Error()
	case ImportOpReadWorking:
		return "failed to read the working staging area: " + e.Err.Error()
	default:
		return e.Err.Error()
	}
}

func (e *ImportError) Unwrap() error {
	return e.Err
}
