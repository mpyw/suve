//go:build production || dev

// Package gui provides the Wails-based GUI application.
package gui

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/provider/aws/infra"
	"github.com/mpyw/suve/internal/provider/azure"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/provider/gcloud"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// =============================================================================
// Provider Registry
// =============================================================================

// registry is the provider registry backing the GUI's read/write operations.
// It mirrors the CLI composition point (internal/cli/commands/internal): AWS
// (param + secret), Google Cloud (secret), and Azure (Key Vault secret + App
// Configuration param) are all registered, so the GUI can browse any backend
// once a scope is selected.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
var registry = func() *provider.Registry {
	reg := aws.NewRegistry()
	gcloud.Register(reg)
	azure.Register(reg)

	return reg
}()

// errInvalidProvider is returned when SelectScope is given an unknown provider.
var errInvalidProvider = stringError("invalid provider: must be 'aws', 'googlecloud', or 'azure'")

// Scope-validation errors surfaced by SelectScope. The wording is
// GUI-appropriate (it names the field, not the CLI flag), since the frontend
// collects these values through form inputs rather than command-line flags.
var (
	// errGoogleCloudProjectRequired is returned when a Google Cloud scope omits
	// the project id.
	errGoogleCloudProjectRequired = stringError("Google Cloud project ID is required")
	// errAzureScopeRequired is returned when an Azure scope specifies neither a
	// Key Vault name nor an App Configuration store name, so no service could be
	// resolved.
	errAzureScopeRequired = stringError("Azure requires a Key Vault name (for secrets) and/or an App Configuration store name (for parameters)")
)

// defaultScope is the initial read/write scope (AWS). The AWS factory builds
// its SSM/Secrets Manager client from the ambient AWS config (region from
// env/profile), so only the Provider field is needed.
//
//nolint:gochecknoglobals // immutable default selector
var defaultScope = provider.Scope{Provider: provider.ProviderAWS}

// =============================================================================
// App Struct
// =============================================================================

// App struct holds application state and dependencies.
//
//nolint:containedctx // Wails apps require storing context from Startup
type App struct {
	ctx context.Context

	// initialProvider is the provider the GUI was launched with (from
	// `suve <provider> --gui`, or the resolved unique-active provider for a bare
	// `suve --gui`). Empty means no explicit choice. Surfaced to the frontend
	// via InitialProvider for the initial selection.
	initialProvider provider.Provider

	// initialService is the service the GUI was launched with ("param" or
	// "secret"), captured from the subcommand carrying `--gui` (e.g.
	// `suve azure param --gui`). Empty means no specific service (launched at the
	// group level or bare). Surfaced to the frontend via InitialService so it can
	// open the matching view.
	initialService string

	// scope is the current read/write provider scope, selected from the
	// frontend via SelectScope. Guarded by scopeMu.
	scope   provider.Scope
	scopeMu sync.RWMutex

	// stagingStore, when non-nil, is returned by getStagingStore verbatim,
	// bypassing scope resolution. It is a test seam (tests inject an in-memory
	// store); production leaves it nil and uses stagingStores.
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
// flag) wins, while an unset one still falls back to env. service is the launch
// service ("param"/"secret", or "" for none) surfaced via InitialService.
func NewApp(initial provider.Scope, service string) *App {
	return &App{
		initialProvider: initial.Provider,
		initialService:  service,
		scope:           hydrateScope(initial),
	}
}

// hydrateScope fills empty resource fields on an initial launch scope from the
// environment. Flag-supplied values take precedence; unset ones fall back to
// GOOGLE_CLOUD_PROJECT / AZURE_KEYVAULT_NAME / AZURE_APPCONFIG_NAME /
// AZURE_APPCONFIG_NAMESPACE. AWS carries no resource field (region comes from
// the ambient AWS config).
func hydrateScope(s provider.Scope) provider.Scope {
	switch s.Provider {
	case provider.ProviderGoogleCloud:
		if s.ProjectID == "" {
			s.ProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
		}
	case provider.ProviderAzure:
		if s.VaultName == "" {
			s.VaultName = os.Getenv("AZURE_KEYVAULT_NAME")
		}

		if s.StoreName == "" {
			s.StoreName = os.Getenv("AZURE_APPCONFIG_NAME")
		}

		if s.AppConfigNamespace == "" {
			s.AppConfigNamespace = os.Getenv("AZURE_APPCONFIG_NAMESPACE")
		}
	case provider.ProviderAWS:
		// region comes from the ambient AWS config; nothing to hydrate.
	default:
		return defaultScope
	}

	return s
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

// ScopeSelection is the frontend-supplied provider + scope for read/write
// operations. Only the fields relevant to the chosen provider are read:
//   - aws: (none; the ambient AWS config supplies the region)
//   - googlecloud: ProjectID
//   - azure: VaultName (Key Vault secret) and/or StoreName (App Configuration
//     param); Namespace is the optional App Configuration namespace (Azure
//     calls it a "label"), applied only to the App Configuration store — empty
//     means the default (null) namespace.
type ScopeSelection struct {
	Provider  string `json:"provider"`
	ProjectID string `json:"projectId"`
	VaultName string `json:"vaultName"`
	StoreName string `json:"storeName"`
	Namespace string `json:"namespace"`
}

// SelectScope sets the current read/write provider scope. It performs no
// network calls; store construction (and any credential resolution) is deferred
// to the next param/secret operation. For Azure the App Configuration namespace
// (sel.Namespace) is carried on the scope; it is only meaningful for the App
// Configuration store and harmless for every other provider/store.
func (a *App) SelectScope(sel ScopeSelection) error {
	scope, err := scopeFromSelection(sel)
	if err != nil {
		return err
	}

	a.scopeMu.Lock()
	a.scope = scope
	a.scopeMu.Unlock()

	return nil
}

// scopeFromSelection maps a frontend selection to a provider.Scope, rejecting
// selections whose required fields are empty. For Azure a single scope carries
// both VaultName and StoreName, so the registry can build either the Key Vault
// (secret) or App Configuration (param) store from it; at least one must be set.
func scopeFromSelection(sel ScopeSelection) (provider.Scope, error) {
	switch provider.Provider(sel.Provider) {
	case provider.ProviderAWS:
		return provider.Scope{Provider: provider.ProviderAWS}, nil
	case provider.ProviderGoogleCloud:
		if sel.ProjectID == "" {
			return provider.Scope{}, errGoogleCloudProjectRequired
		}

		return provider.GoogleCloudScope(sel.ProjectID), nil
	case provider.ProviderAzure:
		if sel.VaultName == "" && sel.StoreName == "" {
			return provider.Scope{}, errAzureScopeRequired
		}

		return provider.Scope{
			Provider:           provider.ProviderAzure,
			VaultName:          sel.VaultName,
			StoreName:          sel.StoreName,
			AppConfigNamespace: sel.Namespace,
		}, nil
	default:
		return provider.Scope{}, fmt.Errorf("%w: %q", errInvalidProvider, sel.Provider)
	}
}

// currentScope returns the active read/write scope.
func (a *App) currentScope() provider.Scope {
	a.scopeMu.RLock()
	defer a.scopeMu.RUnlock()

	return a.scope
}

// GetCurrentScope returns the active read/write scope as a ScopeSelection so the
// frontend can prefill its provider/scope forms (including the env-derived
// initial values from GOOGLE_CLOUD_PROJECT / AZURE_*) instead of silently wiping
// the backend scope on first render.
func (a *App) GetCurrentScope() *ScopeSelection {
	return selectionFromScope(a.currentScope())
}

// EnvScope returns the env-derived scope defaults for an ARBITRARY provider,
// hydrated from the ambient environment (GOOGLE_CLOUD_PROJECT / AZURE_KEYVAULT_NAME
// / AZURE_APPCONFIG_NAME / AZURE_APPCONFIG_NAMESPACE). It is the direct analog of
// the CLI's per-provider env resolution: each provider group reads its own env
// independently of detect, so an explicitly-selected provider always resolves its
// scope from env even in a mixed-env shell. GetCurrentScope only surfaces the
// launch provider's env-derived scope; the frontend calls this to fill the scope
// form for any OTHER provider it switches to. An unknown provider yields the AWS
// default (hydrateScope's fallback).
func (a *App) EnvScope(providerName string) *ScopeSelection {
	return selectionFromScope(hydrateScope(provider.Scope{Provider: provider.Provider(providerName)}))
}

// selectionFromScope is the inverse of scopeFromSelection: it projects a
// provider.Scope back to the frontend DTO. Fields irrelevant to the provider
// stay empty.
func selectionFromScope(s provider.Scope) *ScopeSelection {
	return &ScopeSelection{
		Provider:  string(s.Provider),
		ProjectID: s.ProjectID,
		VaultName: s.VaultName,
		StoreName: s.StoreName,
		Namespace: s.AppConfigNamespace,
	}
}

// stagingScope resolves the provider.Scope that keys on-disk staging state for
// the active provider. It mirrors the CLI's ScopeResolvers: AWS is keyed by the
// STS caller identity (account/region), while Google Cloud and Azure are keyed
// purely from the already-selected scope (project / vault or store) with no
// network call. Deriving the staging store AND the apply/diff
// strategy from this one scope keeps them structurally in sync — the invariant
// that replaces #276's interim non-AWS guard.
func (a *App) stagingScope() (provider.Scope, error) {
	return a.stagingScopeScoped(a.currentScope())
}

// stagingScopeScoped is stagingScope resolved from an already-snapshotted scope,
// so a staging binding can pair its store and strategy against the SAME scope
// even if SelectScope lands between the two resolutions (#560).
func (a *App) stagingScopeScoped(sc provider.Scope) (provider.Scope, error) {
	// Non-AWS scopes already carry their keying fields — no network call.
	if sc.Provider != provider.ProviderAWS {
		return sc, nil
	}
	// An AWS scope that already carries account+region (e.g. injected in tests)
	// needs no STS round-trip.
	if sc.AccountID != "" && sc.Region != "" {
		return sc, nil
	}

	identity, err := infra.GetAWSIdentity(a.ctx)
	if err != nil {
		return provider.Scope{}, err
	}

	return provider.AWSScope(identity.AccountID, identity.Region), nil
}

// stagingScopeForKind resolves the staging scope for ONE service kind. It exists
// because Azure's two services are INDEPENDENT resources with separate staging
// buckets: App Configuration (param) is keyed by store name, Key Vault (secret)
// by vault name, and scope.Key() resolves a combined scope to the Key Vault key
// (VaultName is checked first) — which would silently key App Configuration
// staging under the Key Vault bucket, diverging from the CLI's per-service
// ScopeResolvers (a GUI-staged param would be invisible to `suve azure stage
// param`). Resolving a service-specific scope keeps the GUI and CLI on the exact
// same on-disk key. AWS keeps one account scope for both services (they share
// it); Google Cloud has only the secret service.
func (a *App) stagingScopeForKind(kind provider.Kind) (provider.Scope, error) {
	return a.stagingScopeForKindScoped(a.currentScope(), kind)
}

// stagingScopeForKindScoped is stagingScopeForKind resolved from an
// already-snapshotted scope (#560).
func (a *App) stagingScopeForKindScoped(sc provider.Scope, kind provider.Kind) (provider.Scope, error) {
	if sc.Provider == provider.ProviderAzure {
		if kind == provider.KindParam {
			scope := provider.AzureAppConfigScope(sc.StoreName)
			scope.AppConfigNamespace = sc.AppConfigNamespace

			return scope, nil
		}

		return provider.AzureKeyVaultScope(sc.VaultName), nil
	}

	// AWS (both services share the account scope) and Google Cloud (secret only).
	return a.stagingScopeScoped(sc)
}

// =============================================================================
// Errors
// =============================================================================

// errInvalidService is returned when an invalid service is specified.
var errInvalidService = stringError("invalid service: must be 'param' or 'secret'")

type stringError string

func (e stringError) Error() string { return string(e) }

// =============================================================================
// Helper Methods
// =============================================================================

// paramStore resolves a provider.Store for the parameter service via the
// registry (AWS by default).
func (a *App) paramStore() (provider.Store, error) {
	return a.paramStoreScoped(a.currentScope())
}

// paramStoreScoped is paramStore resolved from an already-snapshotted scope (#560).
func (a *App) paramStoreScoped(sc provider.Scope) (provider.Store, error) {
	return registry.Store(a.ctx, sc, provider.KindParam)
}

// secretStore resolves a provider.Store for the secret service via the
// registry (AWS by default).
func (a *App) secretStore() (provider.Store, error) {
	return a.secretStoreScoped(a.currentScope())
}

// secretStoreScoped is secretStore resolved from an already-snapshotted scope (#560).
func (a *App) secretStoreScoped(sc provider.Scope) (provider.Store, error) {
	return registry.Store(a.ctx, sc, provider.KindSecret)
}

// effectiveParamScope returns the active param scope with the App Configuration
// namespace overridden to ns, so a create/stage can target one concrete
// (key, namespace) without mutating the shared read scope. It is a no-op for
// non-App-Configuration scopes (which have no namespace axis).
func (a *App) effectiveParamScope(ns string) provider.Scope {
	return a.effectiveParamScopeScoped(a.currentScope(), ns)
}

// effectiveParamScopeScoped is effectiveParamScope applied to an
// already-snapshotted scope (#560).
func (a *App) effectiveParamScopeScoped(sc provider.Scope, ns string) provider.Scope {
	if sc.Provider == provider.ProviderAzure && sc.StoreName != "" {
		sc.AppConfigNamespace = ns
	}

	return sc
}

// validateParamNamespace rejects a namespace that names all/multiple namespaces
// (`*` or a `,`-list) for the App Configuration param service — a write targets
// exactly one (key, namespace). It is a no-op for non-App-Configuration scopes
// and for the null/default namespace. Returns the decoded literal namespace.
func (a *App) validateParamNamespace(ns string) (string, error) {
	return a.validateParamNamespaceScoped(a.currentScope(), ns)
}

// validateParamNamespaceScoped is validateParamNamespace resolved from an
// already-snapshotted scope (#560).
func (a *App) validateParamNamespaceScoped(sc provider.Scope, ns string) (string, error) {
	if sc.Provider != provider.ProviderAzure || sc.StoreName == "" {
		return ns, nil
	}

	return aznamespace.Literal(ns)
}

// paramStoreForNamespace resolves a param provider.Store scoped to the given App
// Configuration namespace (no-op namespace for other providers).
func (a *App) paramStoreForNamespace(ns string) (provider.Store, error) {
	return a.paramStoreForNamespaceScoped(a.currentScope(), ns)
}

// paramStoreForNamespaceScoped is paramStoreForNamespace resolved from an
// already-snapshotted scope (#560).
func (a *App) paramStoreForNamespaceScoped(sc provider.Scope, ns string) (provider.Store, error) {
	return registry.Store(a.ctx, a.effectiveParamScopeScoped(sc, ns), provider.KindParam)
}

// appConfigParamStrategyForNamespaceScoped builds the App Configuration staging
// strategy over a provider store scoped to ns, so a staged entry's create/diff/
// apply runs against its own namespace (the per-namespace resolver #431 threads
// into the apply/diff use cases). Resolved from an already-snapshotted scope (#560).
func (a *App) appConfigParamStrategyForNamespaceScoped(sc provider.Scope, ns string) (staging.FullStrategy, error) {
	s, err := a.paramStoreForNamespaceScoped(sc, ns)
	if err != nil {
		return nil, err
	}

	return staging.NewAzureAppConfigParamStrategy(s), nil
}

// isAppConfigParamScope reports whether an already-snapshotted scope is Azure App
// Configuration (the only param service with a namespace axis) (#560).
func isAppConfigParamScope(sc provider.Scope) bool {
	return sc.Provider == provider.ProviderAzure && sc.StoreName != ""
}

// kindForService maps the frontend service string to the provider Kind used to
// resolve the (service-specific) staging scope. An unrecognized service is
// treated as param; getService validates the string separately.
func kindForService(service string) provider.Kind {
	if service == string(staging.ServiceSecret) {
		return provider.KindSecret
	}

	return provider.KindParam
}

func (a *App) getStagingStore(kind provider.Kind) (store.ReadWriteOperator, error) {
	return a.getStagingStoreScoped(a.currentScope(), kind)
}

// getStagingStoreScoped is getStagingStore resolved from an already-snapshotted
// scope, so a binding pairs its store and strategy against the SAME scope (#560).
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

// getParser returns a store-less strategy used to interpret staged entries
// (status/reset): the per-provider strategy so ServiceName/ItemName/delete-option
// semantics match the active provider (e.g. Azure "App Configuration"/"setting",
// no delete options). Selection is by the active provider + service.
func (a *App) getParser(service string) (staging.Parser, error) {
	return a.getParserScoped(a.currentScope(), service)
}

// getParserScoped is getParser resolved from an already-snapshotted scope (#560).
func (a *App) getParserScoped(sc provider.Scope, service string) (staging.Parser, error) {
	switch service {
	case string(staging.ServiceParam):
		if sc.Provider == provider.ProviderAzure {
			return &staging.AzureAppConfigParamStrategy{}, nil
		}

		return &staging.AWSParamStrategy{}, nil
	case string(staging.ServiceSecret):
		switch sc.Provider {
		case provider.ProviderGoogleCloud:
			return &staging.GoogleCloudSecretStrategy{}, nil
		case provider.ProviderAzure:
			return &staging.AzureKeyVaultSecretStrategy{}, nil
		default:
			return &staging.AWSSecretStrategy{}, nil
		}
	default:
		return nil, errInvalidService
	}
}

// serviceStrategyScoped builds the staging strategy for a service, wrapping a
// provider.Store resolved through the registry for the given (already-snapshotted)
// scope. The concrete strategy is provider-specific (AWS SSM/Secrets Manager,
// Google Cloud Secret Manager, Azure Key Vault / App Configuration) and satisfies
// every staging strategy interface, so the typed getters below narrow it as
// needed. It shares the scope with the binding's store, so a staged entry can only
// ever apply to the provider it was staged against (#560).
func (a *App) serviceStrategyScoped(sc provider.Scope, service string) (staging.FullStrategy, error) {
	switch service {
	case string(staging.ServiceParam):
		s, err := a.paramStoreScoped(sc)
		if err != nil {
			return nil, err
		}

		if sc.Provider == provider.ProviderAzure {
			return staging.NewAzureAppConfigParamStrategy(s), nil
		}

		return staging.NewAWSParamStrategy(s), nil
	case string(staging.ServiceSecret):
		s, err := a.secretStoreScoped(sc)
		if err != nil {
			return nil, err
		}

		switch sc.Provider {
		case provider.ProviderGoogleCloud:
			return staging.NewGoogleCloudSecretStrategy(s), nil
		case provider.ProviderAzure:
			return staging.NewAzureKeyVaultSecretStrategy(s), nil
		default:
			return staging.NewAWSSecretStrategy(s), nil
		}
	default:
		return nil, errInvalidService
	}
}

// strategyAsScoped resolves the service strategy for an already-snapshotted scope
// and narrows it to the requested staging strategy interface T. The concrete
// *AWSParamStrategy / *AWSSecretStrategy satisfy every staging strategy interface, so
// this succeeds for the Edit, Apply and Diff interfaces (which FullStrategy
// embeds) as well as for DeleteStrategy (which it does not embed but the concrete
// types implement). It is a free function because Go methods cannot declare type
// parameters (#560).
func strategyAsScoped[T any](a *App, sc provider.Scope, service string) (T, error) {
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

func (a *App) getEditStrategyScoped(sc provider.Scope, service string) (staging.EditStrategy, error) {
	return strategyAsScoped[staging.EditStrategy](a, sc, service)
}

func (a *App) getDeleteStrategyScoped(sc provider.Scope, service string) (staging.DeleteStrategy, error) {
	return strategyAsScoped[staging.DeleteStrategy](a, sc, service)
}

func (a *App) getApplyStrategyScoped(sc provider.Scope, service string) (staging.ApplyStrategy, error) {
	return strategyAsScoped[staging.ApplyStrategy](a, sc, service)
}

func (a *App) getDiffStrategyScoped(sc provider.Scope, service string) (staging.DiffStrategy, error) {
	return strategyAsScoped[staging.DiffStrategy](a, sc, service)
}

// =============================================================================
// AWS Identity
// =============================================================================

// AWSIdentityResult contains AWS account ID, region, and profile for frontend display.
type AWSIdentityResult struct {
	AccountID string `json:"accountId"`
	Region    string `json:"region"`
	Profile   string `json:"profile"`
}

// GetAWSIdentity returns the current AWS account ID, region, and profile.
func (a *App) GetAWSIdentity() (*AWSIdentityResult, error) {
	identity, err := infra.GetAWSIdentity(a.ctx)
	if err != nil {
		return nil, err
	}

	return &AWSIdentityResult{
		AccountID: identity.AccountID,
		Region:    identity.Region,
		Profile:   identity.Profile,
	}, nil
}
