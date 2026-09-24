//declscope:namespace app
//
// sourceFactory is the App's source/mutator seam, consumed only by the
// launch path in run.go. The file split is for readability, not a
// boundary.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	"context"
	"fmt"
	"sync"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/tui/data"
)

// sourceFactory builds the read-path data sources and staging probes for the
// launched scope, resolving provider.Stores through the registry exactly as the
// CLI/GUI do. It caches staging stores per scope key so two stores never race
// the keychain data key (the staging invariant). It is separate from the App so
// the App can be built (with the factory's method as its source seam) before the
// program runs, and so tests can substitute a providermock-backed factory.
type sourceFactory struct {
	ctx   context.Context //nolint:containedctx // the TUI resolves stores lazily against the Run context
	scope provider.Scope

	// resolveIdentity resolves the account-level identity that keys staging for a
	// provider whose launch scope does not carry it (AWS: the STS caller
	// identity). It is a field so tests can substitute a call-counting stub;
	// production wires the provider's binding.DefaultIdentity.
	resolveIdentity binding.IdentityLookup

	mu            sync.Mutex
	stagingStores map[string]store.ReadWriteOperator
	// identityScope memoizes the staging scope resolved by resolveIdentity. The
	// launched provider/scope are fixed for the process lifetime, so the identity
	// is resolved once — not on every staging-store access. A transient failure
	// is not cached, so it retries on the next access.
	identityScope *staging.ResolvedScope
}

// newSourceFactory builds a factory for a launched scope and Run context.
func newSourceFactory(ctx context.Context, scope provider.Scope) *sourceFactory {
	return &sourceFactory{
		ctx:             ctx,
		scope:           scope,
		resolveIdentity: binding.DefaultIdentity(scope.Provider),
		stagingStores:   map[string]store.ReadWriteOperator{},
	}
}

// sourceFor returns the read source and (best-effort) staging probe for a
// service tab, or (nil, nil) when the service is unavailable for the scope.
func (f *sourceFactory) sourceFor(service string) (data.Source, data.StagingProbe) {
	svcCap, ok := capabilityFor(f.scope.Provider, service)
	if !ok {
		return nil, nil
	}

	switch service {
	case string(staging.ServiceParam):
		src := data.NewParamSource(svcCap, f.paramResolver())

		return src, f.stagingProbe(provider.KindParam, service)
	case string(staging.ServiceSecret):
		store, err := registry.Store(f.ctx, f.scope, provider.KindSecret)
		if err != nil {
			return nil, nil
		}

		return data.NewSecretSource(svcCap, store), f.stagingProbe(provider.KindSecret, service)
	default:
		return nil, nil
	}
}

// mutatorFor returns the write-path Mutator for a service tab, or nil when the
// service is unavailable for the scope. It pairs the immediate param/secret use
// cases with the staged-write strategy (from the shared staging binding) and the
// per-scope-cached staging store.
func (f *sourceFactory) mutatorFor(service string) data.Mutator {
	svcCap, ok := capabilityFor(f.scope.Provider, service)
	if !ok {
		return nil
	}

	switch service {
	case string(staging.ServiceParam):
		return data.NewParamMutator(svcCap, f.paramResolver(), f.strategyBuilder(provider.KindParam), f.stagingStoreResolver(svcCap, provider.KindParam))
	case string(staging.ServiceSecret):
		store, err := registry.Store(f.ctx, f.scope, provider.KindSecret)
		if err != nil {
			return nil
		}

		return data.NewSecretMutator(svcCap, store, f.strategyBuilder(provider.KindSecret), f.stagingStoreResolver(svcCap, provider.KindSecret))
	default:
		return nil
	}
}

// stagingService returns the review/apply/reset seam for a service tab, or nil
// when the service is unavailable or has no staging workflow. Resolution of the
// store and strategy is deferred into a StagingResolver so building the page
// never touches the keychain/registry on the update loop; a key-loss surfaces as
// the review's error.
func (f *sourceFactory) stagingService(service string) data.StagingService {
	svcCap, ok := capabilityFor(f.scope.Provider, service)
	if !ok || !svcCap.HasStaging {
		return nil
	}

	switch service {
	case string(staging.ServiceParam):
		return data.NewStagingService(svcCap, svcCap.DisplayName, f.paramStagingResolver())
	case string(staging.ServiceSecret):
		return data.NewStagingService(svcCap, svcCap.DisplayName, f.secretStagingResolver())
	default:
		return nil
	}
}

// paramStagingResolver builds the param service's staging resources: the cached
// staging store paired with the provider-specific strategy, plus a per-namespace
// strategy resolver for Azure App Configuration (whose settings share one store
// across namespaces).
func (f *sourceFactory) paramStagingResolver() data.StagingResolver {
	build := f.strategyBuilder(provider.KindParam)
	resolveStore := f.paramResolver()

	return func(ctx context.Context) (data.StagingResources, error) {
		st, err := f.stagingStore(provider.KindParam)
		if err != nil {
			return data.StagingResources{}, err
		}

		base, err := resolveStore(ctx, "")
		if err != nil {
			return data.StagingResources{}, err
		}

		strategy, err := build(base)
		if err != nil {
			return data.StagingResources{}, err
		}

		res := data.StagingResources{Store: st, Strategy: strategy}

		if b, err := binding.Lookup(f.scope.Provider, provider.KindParam); err == nil && b.Namespaced(f.scope) {
			res.StrategyFor = func(namespace string) (staging.FullStrategy, error) {
				s, err := resolveStore(ctx, namespace)
				if err != nil {
					return nil, err
				}

				return build(s)
			}
		}

		return res, nil
	}
}

// secretStagingResolver builds the secret service's staging resources (no
// namespace axis, so a single strategy handles every entry).
func (f *sourceFactory) secretStagingResolver() data.StagingResolver {
	build := f.strategyBuilder(provider.KindSecret)

	return func(ctx context.Context) (data.StagingResources, error) {
		st, err := f.stagingStore(provider.KindSecret)
		if err != nil {
			return data.StagingResources{}, err
		}

		base, err := registry.Store(ctx, f.scope, provider.KindSecret)
		if err != nil {
			return data.StagingResources{}, err
		}

		strategy, err := build(base)
		if err != nil {
			return data.StagingResources{}, err
		}

		return data.StagingResources{Store: st, Strategy: strategy}, nil
	}
}

// stagingStoreResolver returns a lazy resolver for the service's cached staging
// store, or nil when the service has no staging workflow (so a staged write is
// never offered). Deferring the build keeps dialog open off the keychain.
func (f *sourceFactory) stagingStoreResolver(svcCap capability.ServiceCapability, kind provider.Kind) data.StagingStoreResolver {
	if !svcCap.HasStaging {
		return nil
	}

	return func() (store.ReadWriteOperator, error) {
		return f.stagingStore(kind)
	}
}

// strategyBuilder builds the provider-specific staging strategy for kind over a
// resolved store, through the shared staging binding. A provider without that
// service (or an unknown provider) is an error.
func (f *sourceFactory) strategyBuilder(kind provider.Kind) data.StrategyBuilder {
	return func(s provider.Store) (staging.FullStrategy, error) {
		b, err := binding.Lookup(f.scope.Provider, kind)
		if err != nil {
			return nil, fmt.Errorf("no %s staging strategy: %w", kind, err)
		}

		return b.Strategy(s), nil
	}
}

// paramResolver resolves the param store for a namespace. The binding applies
// the namespace only to a service with a namespace axis (App Configuration).
func (f *sourceFactory) paramResolver() data.StoreResolver {
	return func(ctx context.Context, namespace string) (provider.Store, error) {
		sc := f.scope
		if b, err := binding.Lookup(sc.Provider, provider.KindParam); err == nil {
			sc = b.NamespaceScope(sc, namespace)
		}

		return registry.Store(ctx, sc, provider.KindParam)
	}
}

// stagingProbe returns a lazy, cached staging probe for a service, or nil when
// the service has no staging workflow. Building the on-disk store (which may
// touch the keychain) is deferred to the first probe, off the update loop.
func (f *sourceFactory) stagingProbe(kind provider.Kind, service string) data.StagingProbe {
	svcCap, ok := capabilityFor(f.scope.Provider, service)
	if !ok || !svcCap.HasStaging {
		return nil
	}

	return &lazyStagingProbe{build: func() (data.StagingProbe, error) {
		st, err := f.stagingStore(kind)
		if err != nil {
			return nil, err
		}

		parser, err := parserFor(f.scope.Provider, service)
		if err != nil {
			return nil, err
		}

		return data.NewStagingProbe(parser, st), nil
	}}
}

// stagingStore resolves (and caches) the on-disk staging store for a service
// kind, keyed by the service-specific scope so the key matches the CLI/GUI
// layout and two stores never race the keychain.
func (f *sourceFactory) stagingStore(kind provider.Kind) (store.ReadWriteOperator, error) {
	scope, err := f.stagingScope(kind)
	if err != nil {
		return nil, err
	}

	key := scope.Key()

	f.mu.Lock()
	defer f.mu.Unlock()

	if s := f.stagingStores[key]; s != nil {
		return s, nil
	}

	s, err := file.NewWorkingStore(scope)
	if err != nil {
		return nil, err
	}

	f.stagingStores[key] = s

	return s, nil
}

// stagingScope resolves the service-specific scope that keys staging state
// through the shared staging binding, so the key matches the CLI and GUI.
func (f *sourceFactory) stagingScope(kind provider.Kind) (provider.Scope, error) {
	resolved, err := binding.StagingScope(f.ctx, f.scope, kind, f.memoizedIdentity)
	if err != nil {
		return provider.Scope{}, err
	}

	return resolved.Scope, nil
}

// resolveTarget resolves the launched scope's target for the status bar and the
// apply confirmation. It shares memoizedIdentity with staging, so an AWS launch
// makes one STS call for both.
func (f *sourceFactory) resolveTarget() (provider.Target, error) {
	return binding.ResolveTarget(f.ctx, f.scope, f.memoizedIdentity)
}

// memoizedIdentity resolves (and memoizes) the identity-keyed staging scope.
// Because the launched provider/scope are fixed for the process lifetime, the
// identity is resolved once and reused across every staging-store access rather
// than issuing a fresh network lookup per probe. Only a successful resolution is
// cached, so a transient failure retries.
func (f *sourceFactory) memoizedIdentity(ctx context.Context) (staging.ResolvedScope, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.identityScope != nil {
		return *f.identityScope, nil
	}

	resolved, err := f.resolveIdentity(ctx)
	if err != nil {
		return staging.ResolvedScope{}, err
	}

	f.identityScope = &resolved

	return resolved, nil
}

// lazyStagingProbe defers building the real probe (and thus the on-disk store)
// to the first Staged call, so page construction never blocks on the keychain.
type lazyStagingProbe struct {
	build func() (data.StagingProbe, error)
}

func (p *lazyStagingProbe) Staged(ctx context.Context) (data.StagingSnapshot, error) {
	probe, err := p.build()
	if err != nil {
		// A build failure is a store-construction hard-fail (e.g. a keychain
		// key-loss while encrypted state exists). Mark it so the browser surfaces it
		// on the error line instead of silently dropping the staging badges.
		return data.StagingSnapshot{}, &data.StoreUnavailableError{Err: err}
	}

	return probe.Staged(ctx)
}

// capabilityFor looks up the ServiceCapability for a provider+service in the
// neutral matrix, so every gate reads one source of truth.
func capabilityFor(prov provider.Provider, service string) (capability.ServiceCapability, bool) {
	for _, pc := range capability.All() {
		if pc.Provider != string(prov) {
			continue
		}

		for _, sc := range pc.Services {
			if sc.Service == service {
				return sc, true
			}
		}
	}

	return capability.ServiceCapability{}, false
}

// parserFor returns the store-less staging parser for a provider+service
// through the shared staging binding. An unknown provider, or one that does not
// offer the service, is an error.
func parserFor(prov provider.Provider, service string) (staging.Parser, error) {
	b, err := binding.Lookup(prov, provider.Kind(service))
	if err != nil {
		return nil, err
	}

	return b.Parser(), nil
}
