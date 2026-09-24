package provider

import (
	"context"
	"fmt"
)

// Registry maps a Provider to its Factory.
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
