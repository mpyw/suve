package staging

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/provider"
)

// ErrServiceNotConfigured is returned by a ScopeResolver when the active scope
// does not name this service's backing resource (e.g. no Azure Key Vault while
// only App Configuration is configured). A single-service command treats it as
// a fatal usage error (with the resolver's descriptive message); a provider-wide
// command treats it as "skip this service" — an unconfigured service can hold no
// staged state, since staging is keyed by the resource name.
var ErrServiceNotConfigured = errors.New("staging service not configured")

// ResolvedScope is the outcome of resolving the active provider's staging scope:
// the provider.Scope used to key on-disk staging state, plus a human-readable
// Target line shown in apply/pop confirmation prompts (e.g. an AWS
// profile/account/region, or a Google Cloud project).
type ResolvedScope struct {
	// Scope keys the on-disk staging state (see provider.Scope.Key).
	Scope provider.Scope
	// Target is a human-readable description of where changes will be applied.
	Target string
}

// ScopeResolver resolves the active provider's staging scope. AWS resolves it
// from the STS caller identity; Google Cloud from the configured project. It may
// perform network calls (e.g. STS GetCallerIdentity), so it is only invoked by
// staging commands, never by read/write commands.
type ScopeResolver func(ctx context.Context) (ResolvedScope, error)

// KindToService maps a provider Kind to the equivalent staging Service.
func KindToService(k provider.Kind) Service {
	switch k {
	case provider.KindParam:
		return ServiceParam
	case provider.KindSecret:
		return ServiceSecret
	default:
		return Service(k)
	}
}

// SupportedServices returns the staging Services supported by the given scope,
// in the scope's stable kind order. This is the registry-driven iteration
// source that replaces hardcoded {ServiceParam, ServiceSecret} loops.
func SupportedServices(scope provider.Scope) []Service {
	kinds := scope.SupportedKinds()

	services := make([]Service, 0, len(kinds))
	for _, k := range kinds {
		services = append(services, KindToService(k))
	}

	return services
}
