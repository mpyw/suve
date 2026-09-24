// client.go is this package's subject: the shared registry and the staging
// wiring every provider's commands go through. Core, so that internal.Store
// does not have to become ClientStore.
//declscope:core

package internal

import (
	"context"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/builtin"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
)

// registry is the provider registry reachable by every CLI command, built by
// builtin.NewRegistry (the composition the GUI and TUI share). Provider command
// packages build their own provider.Scope and resolve stores through Store.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
var registry = builtin.NewRegistry()

// Store resolves a provider.Store for scope and kind through the shared
// registry. Each provider's command packages build the scope from their own
// flags and context, then call this.
func Store(ctx context.Context, scope provider.Scope, kind provider.Kind) (provider.Store, error) {
	return registry.Store(ctx, scope, kind)
}

// StrategyFactory returns the staging strategy factory for a provider and kind:
// it resolves the store with store and wraps it in the strategy from the shared
// staging binding (internal/staging/binding), the same lookup the GUI and TUI
// use. It panics for a provider/kind without a binding, which is a wiring bug in
// a static command config.
func StrategyFactory(
	p provider.Provider,
	kind provider.Kind,
	store func(context.Context) (provider.Store, error),
) staging.StrategyFactory {
	b := mustBinding(p, kind)

	return func(ctx context.Context) (staging.FullStrategy, error) {
		s, err := store(ctx)
		if err != nil {
			return nil, err
		}

		return b.Strategy(s), nil
	}
}

// ParserFactory returns the store-less parser factory for a provider and kind
// from the shared staging binding. It panics for a provider/kind without a
// binding, which is a wiring bug in a static command config.
func ParserFactory(p provider.Provider, kind provider.Kind) staging.ParserFactory {
	return mustBinding(p, kind).ParserFactory()
}

// mustBinding looks up a staging binding for static command wiring.
func mustBinding(p provider.Provider, kind provider.Kind) binding.Binding {
	b, err := binding.Lookup(p, kind)
	if err != nil {
		panic(err)
	}

	return b
}
