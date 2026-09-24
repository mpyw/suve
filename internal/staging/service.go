package staging

import (
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider"
)

// Service is the provider-neutral service axis a staged change belongs to.
type Service string

const (
	// ServiceParam is the parameter service (e.g. AWS SSM Parameter Store,
	// Azure App Configuration).
	ServiceParam Service = "param"
	// ServiceSecret is the secret service (e.g. AWS Secrets Manager, Google
	// Cloud Secret Manager, Azure Key Vault).
	ServiceSecret Service = "secret"
)

// secretServiceItemName is the display item name shared by the secret staging
// strategies (AWS Secrets Manager, Google Cloud Secret Manager, Azure Key
// Vault). Centralizing it avoids repeating the literal across strategies.
//
//declscope:package // shared by design across the per-provider secret strategy files
const secretServiceItemName = "secret"

// ServiceStrategy defines the common interface for service-specific operations.
// Every provider's staging strategy implements it, so the stage commands are
// written once for all services.
type ServiceStrategy interface {
	// Service returns the service type (ServiceParam or ServiceSecret).
	Service() Service

	// ServiceName returns the user-friendly service name (e.g., "SSM Parameter Store", "Key Vault").
	ServiceName() string

	// ItemName returns the item name for messages (e.g., "parameter", "secret").
	ItemName() string

	// HasDeleteOptions returns true if delete options should be displayed.
	HasDeleteOptions() bool
}

// ErrServiceNotConfigured is returned by a ScopeResolver when the active scope
// does not name this service's backing resource (e.g. no Azure Key Vault while
// only App Configuration is configured). A single-service command treats it as
// a fatal usage error (with the resolver's descriptive message); a provider-wide
// command treats it as "skip this service" — an unconfigured service can hold no
// staged state, since staging is keyed by the resource name.
var ErrServiceNotConfigured = errors.New("staging service not configured")

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

	services := lo.Map(kinds, func(k provider.Kind, _ int) Service {
		return KindToService(k)
	})

	return services
}
