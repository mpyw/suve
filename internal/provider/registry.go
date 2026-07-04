package provider

import (
	"context"
	"fmt"
)

// Provider identifies a cloud provider backend.
type Provider string

// ProviderAWS is the Amazon Web Services provider.
const ProviderAWS Provider = "aws"

// Kind selects a store kind within a provider (some providers offer only one).
type Kind string

const (
	// KindParam selects a parameter store (e.g. AWS SSM Parameter Store).
	KindParam Kind = "param"
	// KindSecret selects a secret store (e.g. AWS Secrets Manager).
	KindSecret Kind = "secret"
)

// Scope identifies a provider-specific namespace. The full multi-field Scope
// (and scope-keyed storage) is ported in #200; this is the minimal contract
// the registry needs.
type Scope struct {
	// Provider selects which backend the scope belongs to.
	Provider Provider
	// AccountID is the AWS account id (AWS).
	AccountID string // AWS
	// Region is the AWS region (AWS).
	Region string // AWS
}

// Factory builds a Store for a scope + kind. It returns ErrUnsupportedKind if
// the provider does not offer that kind (e.g. GCP has no param store).
type Factory interface {
	// Store builds a Store for the given scope and kind.
	Store(ctx context.Context, scope Scope, kind Kind) (Store, error)
}

// ErrUnsupportedKind is returned when a provider does not offer the requested store kind.
var ErrUnsupportedKind = fmt.Errorf("provider: unsupported store kind")

// ErrNoFactory is returned when no factory is registered for a provider.
var ErrNoFactory = fmt.Errorf("provider: no factory registered for provider")

// Registry maps a Provider to its Factory, replacing direct infra.NewXClient calls.
type Registry struct{ factories map[Provider]Factory }

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{factories: map[Provider]Factory{}} }

// Register associates a Factory with a Provider, overwriting any prior registration.
func (r *Registry) Register(p Provider, f Factory) { r.factories[p] = f }

// Store resolves the factory for scope.Provider and builds the requested store.
func (r *Registry) Store(ctx context.Context, scope Scope, kind Kind) (Store, error) {
	f, ok := r.factories[scope.Provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoFactory, scope.Provider)
	}

	return f.Store(ctx, scope, kind)
}
