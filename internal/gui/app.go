//go:build production || dev

// App is the Wails-bound type this package is named for, together with the
// store, parser and strategy resolution every binding file goes through. The
// per-service binding files (param, secret, staging, ...) are its methods by
// concern, so app.go is the core they build on.
//declscope:core

// Package gui provides the Wails-based GUI application.
package gui

import (
	"context"
	"fmt"
	"sync"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/namespaces"
	"github.com/mpyw/suve/internal/provider/builtin"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// registry is the provider registry backing the GUI's read/write operations.
// It is built by builtin.NewRegistry, the same composition the CLI and TUI use,
// so the GUI can browse any backend once a scope is selected.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
//declscope:package // shared with the param, secret and staging namespaces
var registry = builtin.NewRegistry()

// errInvalidProvider is returned when SelectScope is given an unknown provider.
//
//declscope:package // shared with the staging namespace
var errInvalidProvider = stringError("invalid provider: must be 'aws', 'googlecloud', or 'azure'")

// App struct holds application state and dependencies.
//
//nolint:containedctx // Wails apps require storing context from Startup
type App struct {
	//declscope:package // shared with the param, secret and staging namespaces
	ctx context.Context

	// initialProvider is the provider the GUI was launched with (from
	// `suve <provider> --gui`, or the resolved unique-active provider for a bare
	// `suve --gui`). Empty means no explicit choice. Surfaced to the frontend
	// via InitialProvider for the initial selection.
	//
	//declscope:package // shared with the detect namespace
	initialProvider provider.Provider

	// initialService is the service the GUI was launched with ("param" or
	// "secret"), captured from the subcommand carrying `--gui` (e.g.
	// `suve azure param --gui`). Empty means no specific service (launched at the
	// group level or bare). Surfaced to the frontend via InitialService so it can
	// open the matching view.
	initialService string

	// scope is the current read/write provider scope, selected from the
	// frontend via SelectScope. It is the zero Scope (no provider) until a
	// provider is chosen. Guarded by scopeMu.
	//
	//declscope:package // shared with the param, secret and staging namespaces
	scope provider.Scope
	// scopeMu guards scope.
	//
	//declscope:package // SelectScope and currentScope in scope.go take it
	scopeMu sync.RWMutex

	// stagingStore, when non-nil, is returned by getStagingStore verbatim,
	// bypassing scope resolution. It is a test seam (tests inject an in-memory
	// store); production leaves it nil and uses stagingStores.
	//
	//declscope:package // shared with the staging namespace
	stagingStore store.ReadWriteOperator

	// stagingStores holds the working staging areas (backed by
	// param.json/secret.json), keyed by provider.Scope.Key() so each
	// provider/scope has isolated staging state (matching the CLI's
	// ~/.suve/staging/{scope.Key()} layout).
	stagingStores  map[string]store.ReadWriteOperator
	stagingStoreMu sync.Mutex // protects stagingStore + stagingStores
}

// NewApp creates a new App with the given initial launch scope and service.
// Empty resource fields on the scope are hydrated from the ambient environment,
// so an explicit selection (e.g. from a --project / --vault-name / --store-name
// flag) wins, while an unset one still falls back to env. A zero Provider means
// no provider is selected yet: the scope stays zero until SelectScope. An
// unknown provider is an error. service is the launch service
// ("param"/"secret", or "" for none) surfaced via InitialService.
func NewApp(initial provider.Scope, service string) (*App, error) {
	scope := initial
	if initial.Provider != "" {
		var err error

		scope, err = hydrateScope(initial)
		if err != nil {
			return nil, err
		}
	}

	return &App{
		initialProvider: initial.Provider,
		initialService:  service,
		scope:           scope,
	}, nil
}

// InitialProvider returns the provider the GUI was launched with (empty when no
// explicit `--gui` provider was chosen), so the frontend can pre-select it.
func (a *App) InitialProvider() string {
	return string(a.initialProvider)
}

// InitialService returns the service the GUI was launched with ("param" or
// "secret"), or "" when no specific service was chosen (group-level or bare
// `--gui`), so the frontend can open the matching view.
func (a *App) InitialService() string {
	return a.initialService
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// errInvalidService is returned when an invalid service is specified.
//
//declscope:package // shared with the staging namespace
var errInvalidService = stringError("invalid service: must be 'param' or 'secret'")

// errUnsupportedService is returned when the selected provider (or no provider)
// does not offer the requested service.
//
//declscope:package // shared with the staging namespace
var errUnsupportedService = stringError("service is not offered by the selected provider")

//declscope:package // shared with the secret and staging namespaces
type stringError string

func (e stringError) Error() string { return string(e) }

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
// scope (#560) and narrows it to the requested staging strategy interface T.
// Every provider's concrete strategy satisfies every staging strategy
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
