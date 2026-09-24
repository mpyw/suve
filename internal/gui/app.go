//go:build production || dev

// App is the Wails-bound type this package is named for. The per-service
// binding files (param, secret, staging, ...) are its methods by concern, so
// app.go is the core they build on.
//declscope:core

// Package gui provides the Wails-based GUI application.
package gui

import (
	"context"
	"sync"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/builtin"
	"github.com/mpyw/suve/internal/staging/store"
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
