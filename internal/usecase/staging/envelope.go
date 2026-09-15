package staging

import (
	"context"

	"github.com/mpyw/suve/internal/staging"
)

// EnvelopeWriter writes a single service's staged state to an export target
// (typically a per-service envelope file). Adapters bind the destination path,
// scope, and passphrase; the use case only supplies the service and its state.
type EnvelopeWriter interface {
	// WriteEnvelope serializes state (scoped to svc) to the export target.
	WriteEnvelope(ctx context.Context, svc staging.Service, state *staging.State) error
}

// EnvelopeReader reads a single service's staged state from an import source
// (typically a per-service envelope file). Adapters bind the source path, scope
// validation, and passphrase; the use case only supplies the service. For a
// missing file in the directory/global case the adapter returns an empty state
// with a nil error (an absent service is skipped, not an error).
type EnvelopeReader interface {
	// ReadState returns the decoded state for svc.
	ReadState(ctx context.Context, svc staging.Service) (*staging.State, error)
}
