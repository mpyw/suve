package staging

import (
	"context"

	"github.com/mpyw/suve/internal/provider"
)

// ResolvedScope is the outcome of resolving the active provider's staging scope:
// the provider.Scope used to key on-disk staging state, plus the Target shown in
// apply/import confirmation prompts (e.g. an AWS profile/account/region, or a
// Google Cloud project).
type ResolvedScope struct {
	// Scope keys the on-disk staging state (see provider.Scope.Key).
	Scope provider.Scope
	// Target describes where changes will be applied.
	Target provider.Target
}

// ScopeResolver resolves the active provider's staging scope. AWS resolves it
// from the STS caller identity; Google Cloud from the configured project. It may
// perform network calls (e.g. STS GetCallerIdentity), so it is only invoked by
// staging commands, never by read/write commands.
type ScopeResolver func(ctx context.Context) (ResolvedScope, error)
