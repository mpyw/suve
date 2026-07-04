package staging

import "github.com/mpyw/suve/internal/provider"

// ServiceToKind maps a staging Service to the equivalent provider Kind.
func ServiceToKind(s Service) provider.Kind {
	switch s {
	case ServiceParam:
		return provider.KindParam
	case ServiceSecret:
		return provider.KindSecret
	default:
		return provider.Kind(s)
	}
}

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
