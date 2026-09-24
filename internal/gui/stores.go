//go:build production || dev

// The App's store, parser and strategy resolution for the current (or a
// snapshotted) scope. Every binding file resolves through these, so this file
// is part of app.go's core rather than a namespace of its own.
//declscope:core

package gui

import (
	"fmt"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/namespaces"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// paramStore resolves a provider.Store for the parameter service via the
// registry for the current scope.
//
//declscope:package // shared with the param namespace
func (a *App) paramStore() (provider.Store, error) {
	return a.paramStoreScoped(a.currentScope())
}

// paramStoreScoped is paramStore resolved from an already-snapshotted scope (#560).
func (a *App) paramStoreScoped(sc provider.Scope) (provider.Store, error) {
	return registry.Store(a.ctx, sc, provider.KindParam)
}

// secretStore resolves a provider.Store for the secret service via the
// registry for the current scope.
//
//declscope:package // shared with the secret namespace
func (a *App) secretStore() (provider.Store, error) {
	return a.secretStoreScoped(a.currentScope())
}

// secretStoreScoped is secretStore resolved from an already-snapshotted scope (#560).
func (a *App) secretStoreScoped(sc provider.Scope) (provider.Store, error) {
	return registry.Store(a.ctx, sc, provider.KindSecret)
}

// effectiveParamScopeScoped returns an already-snapshotted param scope (#560)
// with its namespace overridden to ns, so a create/stage can target one
// concrete (key, namespace) without mutating the shared read scope. The binding
// applies it only to a service with a namespace axis (App Configuration).
//
//declscope:package // shared with the param and staging namespaces
func (a *App) effectiveParamScopeScoped(sc provider.Scope, ns string) provider.Scope {
	b, err := binding.Lookup(sc.Provider, provider.KindParam)
	if err != nil {
		return sc
	}

	return b.NamespaceScope(sc, ns)
}

// validateParamNamespace rejects a namespace that names all/multiple namespaces
// (`*` or a `,`-list) for the App Configuration param service — a write targets
// exactly one (key, namespace). It is a no-op for non-App-Configuration scopes
// and for the null/default namespace. Returns the decoded literal namespace.
//
//declscope:package // shared with the param namespace
func (a *App) validateParamNamespace(ns string) (string, error) {
	return a.validateParamNamespaceScoped(a.currentScope(), ns)
}

// validateParamNamespaceScoped is validateParamNamespace resolved from an
// already-snapshotted scope (#560).
//
//declscope:package // shared with the staging namespace
func (a *App) validateParamNamespaceScoped(sc provider.Scope, ns string) (string, error) {
	if !hasParamNamespaces(sc) {
		return ns, nil
	}

	return namespaces.Literal(ns)
}

// paramStoreForNamespace resolves a param provider.Store scoped to the given App
// Configuration namespace (no-op namespace for other providers).
//
//declscope:package // shared with the param namespace
func (a *App) paramStoreForNamespace(ns string) (provider.Store, error) {
	return a.paramStoreForNamespaceScoped(a.currentScope(), ns)
}

// paramStoreForNamespaceScoped is paramStoreForNamespace resolved from an
// already-snapshotted scope (#560).
func (a *App) paramStoreForNamespaceScoped(sc provider.Scope, ns string) (provider.Store, error) {
	return registry.Store(a.ctx, a.effectiveParamScopeScoped(sc, ns), provider.KindParam)
}

// paramStrategyForNamespaceScoped builds the param staging strategy over a
// provider store scoped to ns, so a staged entry's create/diff/apply runs
// against its own namespace (the per-namespace resolver #431 threads into the
// apply/diff use cases). Resolved from an already-snapshotted scope (#560).
//
//declscope:package // shared with the staging namespace
func (a *App) paramStrategyForNamespaceScoped(sc provider.Scope, ns string) (staging.FullStrategy, error) {
	b, err := a.stagingBinding(sc, string(staging.ServiceParam))
	if err != nil {
		return nil, err
	}

	s, err := a.paramStoreForNamespaceScoped(sc, ns)
	if err != nil {
		return nil, err
	}

	return b.Strategy(s), nil
}

// hasParamNamespaces reports whether an already-snapshotted scope's param
// service has a namespace axis (Azure App Configuration) (#560).
//
//declscope:package // shared with the staging namespace
func hasParamNamespaces(sc provider.Scope) bool {
	b, err := binding.Lookup(sc.Provider, provider.KindParam)

	return err == nil && b.Namespaced(sc)
}

// kindForService maps the frontend service string to the provider Kind used to
// resolve the (service-specific) staging scope. An unrecognized service is
// treated as param; getService validates the string separately.
//
//declscope:package // shared with the staging namespace
func kindForService(service string) provider.Kind {
	if service == string(staging.ServiceSecret) {
		return provider.KindSecret
	}

	return provider.KindParam
}

//declscope:package // shared with the staging namespace
func (a *App) getStagingStore(kind provider.Kind) (store.ReadWriteOperator, error) {
	return a.getStagingStoreScoped(a.currentScope(), kind)
}

// getStagingStoreScoped is getStagingStore resolved from an already-snapshotted
// scope, so a binding pairs its store and strategy against the SAME scope (#560).
//
//declscope:package // shared with the staging namespace
func (a *App) getStagingStoreScoped(sc provider.Scope, kind provider.Kind) (store.ReadWriteOperator, error) {
	// Test seam: an injected store bypasses scope resolution (and any STS call).
	a.stagingStoreMu.Lock()
	if a.stagingStore != nil {
		defer a.stagingStoreMu.Unlock()

		return a.stagingStore, nil
	}
	a.stagingStoreMu.Unlock()

	scope, err := a.stagingScopeForKindScoped(sc, kind)
	if err != nil {
		return nil, err
	}

	key := scope.Key()

	a.stagingStoreMu.Lock()
	defer a.stagingStoreMu.Unlock()

	if s := a.stagingStores[key]; s != nil {
		return s, nil
	}

	s, err := file.NewWorkingStore(scope)
	if err != nil {
		return nil, err
	}

	if a.stagingStores == nil {
		a.stagingStores = make(map[string]store.ReadWriteOperator)
	}

	a.stagingStores[key] = s

	return s, nil
}

//declscope:package // shared with the staging namespace
func (a *App) getService(service string) (staging.Service, error) {
	switch service {
	case string(staging.ServiceParam):
		return staging.ServiceParam, nil
	case string(staging.ServiceSecret):
		return staging.ServiceSecret, nil
	default:
		return "", errInvalidService
	}
}

// stagingBinding looks up the shared staging binding for sc's provider and a
// frontend service string. An unknown provider, or one that does not offer the
// service, is errUnsupportedService.
//
//declscope:package // shared with the spec namespace
func (a *App) stagingBinding(sc provider.Scope, service string) (binding.Binding, error) {
	if _, err := a.getService(service); err != nil {
		return binding.Binding{}, err
	}

	b, err := binding.Lookup(sc.Provider, provider.Kind(service))
	if err != nil {
		return binding.Binding{}, fmt.Errorf("%w: provider %q, service %q", errUnsupportedService, sc.Provider, service)
	}

	return b, nil
}

// getParserScoped returns a store-less strategy used to interpret staged
// entries (status/reset) for an already-snapshotted scope (#560): the
// per-provider parser from the shared staging binding, so ServiceName/ItemName/
// delete-option semantics match the provider (e.g. Azure "App Configuration"/
// "setting", no delete options). A provider that does not offer the service is
// an error.
//
//declscope:package // shared with the staging namespace
func (a *App) getParserScoped(sc provider.Scope, service string) (staging.Parser, error) {
	b, err := a.stagingBinding(sc, service)
	if err != nil {
		return nil, err
	}

	return b.Parser(), nil
}

// serviceStrategyScoped builds the staging strategy for a service, wrapping a
// provider.Store resolved through the registry for the given (already-snapshotted)
// scope. The concrete strategy comes from the shared staging binding and
// satisfies every staging strategy interface, so the typed getters below narrow
// it as needed. It shares the scope with the binding's store, so a staged entry
// can only ever apply to the provider it was staged against (#560).
//
//declscope:package // shared with the staging namespace
func (a *App) serviceStrategyScoped(sc provider.Scope, service string) (staging.FullStrategy, error) {
	b, err := a.stagingBinding(sc, service)
	if err != nil {
		return nil, err
	}

	s, err := registry.Store(a.ctx, sc, provider.Kind(service))
	if err != nil {
		return nil, err
	}

	return b.Strategy(s), nil
}

// strategyAsScoped resolves the service strategy for an already-snapshotted
// scope (#560) and narrows it to the requested staging strategy interface T. The
// concrete *AWSParamStrategy / *AWSSecretStrategy satisfy every staging strategy
// interface, so this succeeds for the Edit, Apply and Diff interfaces (which
// FullStrategy embeds) as well as for DeleteStrategy (which it does not embed
// but the concrete types implement).
//
// Callers instantiate T at the call site, so no per-interface wrapper method is
// needed:
//
//	strategy, err := a.strategyAsScoped[staging.EditStrategy](sc, service)
//
//declscope:package // shared with the staging namespace
func (a *App) strategyAsScoped[T any](sc provider.Scope, service string) (T, error) {
	var zero T

	strategy, err := a.serviceStrategyScoped(sc, service)
	if err != nil {
		return zero, err
	}

	narrowed, ok := any(strategy).(T)
	if !ok {
		return zero, errInvalidService
	}

	return narrowed, nil
}
