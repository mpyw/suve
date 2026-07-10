package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// Export error Op codes.
const (
	exportOpLoad  = "load"
	exportOpWrite = "write"
	exportOpClear = "clear"
)

// EnvelopeWriter writes a single service's staged state to an export target
// (typically a per-service envelope file). Adapters bind the destination path,
// scope, and passphrase; the use case only supplies the service and its state.
type EnvelopeWriter interface {
	// WriteEnvelope serializes state (scoped to svc) to the export target.
	WriteEnvelope(ctx context.Context, svc staging.Service, state *staging.State) error
}

// ExportInput holds input for the export use case.
type ExportInput struct {
	// Service filters the export to a specific service. Empty means all services
	// that have staged changes.
	Service staging.Service
	// Keep preserves the working staging area after exporting.
	Keep bool
}

// ExportOutput holds the result of the export use case.
type ExportOutput struct {
	// EntryCount is the number of entries exported.
	EntryCount int
	// TagCount is the number of tag entries exported.
	TagCount int
}

// ExportUseCase writes the working staging area out to an export target
// wholesale. Unlike the former stash push, export never merges with existing
// destination data: writing state out is a serialization of the current working
// area, not a reconciliation with whatever a file previously held.
type ExportUseCase struct {
	// Working is the working staging area (param.json/secret.json).
	Working store.FileStore
	// Target receives the exported per-service state.
	Target EnvelopeWriter
}

// Execute runs the export use case.
func (u *ExportUseCase) Execute(ctx context.Context, input ExportInput) (*ExportOutput, error) {
	// Read the working staging area (keep=true; we clear it afterwards only when
	// the write succeeded and --keep was not requested).
	workingState, err := u.Working.Drain(ctx, "", true)
	if err != nil {
		return nil, &ExportError{Op: exportOpLoad, Err: err}
	}

	// Determine which services actually have data to export, along with their
	// extracted per-service state. A service filter narrows to that one service;
	// otherwise every non-empty service is written.
	targets := exportTargets(input.Service, workingState)
	if len(targets) == 0 {
		return nil, ErrNothingToExport
	}

	output := &ExportOutput{}

	for _, t := range targets {
		if err := u.Target.WriteEnvelope(ctx, t.service, t.state); err != nil {
			return nil, &ExportError{Op: exportOpWrite, Err: err}
		}

		output.EntryCount += t.state.EntryCount()
		output.TagCount += t.state.TagCount()
	}

	// Clear the exported working area unless --keep is specified. The export has
	// already succeeded, so a failure here is non-fatal.
	if !input.Keep {
		if input.Service != "" {
			workingState.RemoveService(input.Service)
		} else {
			workingState = staging.NewEmptyState()
		}

		if err := u.Working.WriteState(ctx, "", workingState); err != nil {
			return output, &ExportError{Op: exportOpClear, Err: err, NonFatal: true}
		}
	}

	return output, nil
}

// exportTarget pairs a service with its extracted single-service state.
type exportTarget struct {
	service staging.Service
	state   *staging.State
}

// exportTargets returns the services that should be exported together with their
// extracted state. With a service filter it is that single service when it has
// data; otherwise it is every service (param, secret) whose state is non-empty.
func exportTargets(service staging.Service, state *staging.State) []exportTarget {
	services := []staging.Service{staging.ServiceParam, staging.ServiceSecret}
	if service != "" {
		services = []staging.Service{service}
	}

	var targets []exportTarget

	for _, svc := range services {
		svcState := state.ExtractService(svc)
		if svcState.IsEmpty() {
			continue
		}

		targets = append(targets, exportTarget{service: svc, state: svcState})
	}

	return targets
}

var (
	// ErrNothingToExport is returned when there are no staged changes to export.
	ErrNothingToExport = errors.New("no staged changes to export")
)

// ExportError represents an error during an export operation.
type ExportError struct {
	Op       string // "load", "write", "clear"
	Err      error
	NonFatal bool // If true, the error is non-fatal (state was already written)
}

func (e *ExportError) Error() string {
	switch e.Op {
	case exportOpLoad:
		return "failed to read the working staging area: " + e.Err.Error()
	case exportOpWrite:
		return "failed to write export file: " + e.Err.Error()
	case exportOpClear:
		return "failed to clear the working staging area: " + e.Err.Error()
	default:
		return e.Err.Error()
	}
}

func (e *ExportError) Unwrap() error {
	return e.Err
}
