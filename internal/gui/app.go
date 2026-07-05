//go:build production || dev

// Package gui provides the Wails-based GUI application.
package gui

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/provider/azure"
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

	// scope is the current read/write provider scope, selected from the
	// frontend via SelectScope. Guarded by scopeMu.
	scope   provider.Scope
	scopeMu sync.RWMutex

	// Staging store (the working staging area, backed by param.json/secret.json)
	stagingStore   store.ReadWriteOperator
	stagingStoreMu sync.Mutex // protects stagingStore initialization
}

// NewApp creates a new App with the given initial provider. The initial
// read/write scope is derived from that provider using the ambient environment
// (project / subscription / resource-group / vault / store); an empty provider
// defaults to AWS.
func NewApp(initial provider.Provider) *App {
	return &App{
		initialProvider: initial,
		scope:           initialScope(initial),
	}
}

// initialScope derives the launch scope for a provider from the environment.
func initialScope(p provider.Provider) provider.Scope {
	switch p {
	case provider.ProviderGoogleCloud:
		return provider.GoogleCloudScope(os.Getenv("GOOGLE_CLOUD_PROJECT"))
	case provider.ProviderAzure:
		return provider.Scope{
			Provider:       provider.ProviderAzure,
			SubscriptionID: os.Getenv("AZURE_SUBSCRIPTION_ID"),
			ResourceGroup:  os.Getenv("AZURE_RESOURCE_GROUP"),
			VaultName:      os.Getenv("AZURE_KEYVAULT_NAME"),
			StoreName:      os.Getenv("AZURE_APPCONFIG_NAME"),
		}
	case provider.ProviderAWS:
		return provider.Scope{Provider: provider.ProviderAWS}
	default:
		return defaultScope
	}
}

// InitialProvider returns the provider the GUI was launched with (empty when no
// explicit `--gui` provider was chosen), so the frontend can pre-select it.
func (a *App) InitialProvider() string {
	return string(a.initialProvider)
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// ScopeSelection is the frontend-supplied provider + scope for read/write
// operations. Only the fields relevant to the chosen provider are read:
//   - aws: (none; the ambient AWS config supplies the region)
//   - googlecloud: ProjectID
//   - azure: SubscriptionID + ResourceGroup, plus VaultName (Key Vault secret)
//     and/or StoreName (App Configuration param)
type ScopeSelection struct {
	Provider       string `json:"provider"`
	ProjectID      string `json:"projectId"`
	SubscriptionID string `json:"subscriptionId"`
	ResourceGroup  string `json:"resourceGroup"`
	VaultName      string `json:"vaultName"`
	StoreName      string `json:"storeName"`
}

// SelectScope sets the current read/write provider scope. It performs no
// network calls; store construction (and any credential resolution) is deferred
// to the next param/secret operation.
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

// scopeFromSelection maps a frontend selection to a provider.Scope. For Azure a
// single scope carries both VaultName and StoreName, so the registry can build
// either the Key Vault (secret) or App Configuration (param) store from it.
func scopeFromSelection(sel ScopeSelection) (provider.Scope, error) {
	switch provider.Provider(sel.Provider) {
	case provider.ProviderAWS:
		return provider.Scope{Provider: provider.ProviderAWS}, nil
	case provider.ProviderGoogleCloud:
		return provider.GoogleCloudScope(sel.ProjectID), nil
	case provider.ProviderAzure:
		return provider.Scope{
			Provider:       provider.ProviderAzure,
			SubscriptionID: sel.SubscriptionID,
			ResourceGroup:  sel.ResourceGroup,
			VaultName:      sel.VaultName,
			StoreName:      sel.StoreName,
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
	return registry.Store(a.ctx, a.currentScope(), provider.KindParam)
}

// secretStore resolves a provider.Store for the secret service via the
// registry (AWS by default).
func (a *App) secretStore() (provider.Store, error) {
	return registry.Store(a.ctx, a.currentScope(), provider.KindSecret)
}

func (a *App) getStagingStore() (store.ReadWriteOperator, error) {
	a.stagingStoreMu.Lock()
	defer a.stagingStoreMu.Unlock()

	if a.stagingStore != nil {
		return a.stagingStore, nil
	}

	identity, err := infra.GetAWSIdentity(a.ctx)
	if err != nil {
		return nil, err
	}

	s, err := file.NewWorkingStore(provider.AWSScope(identity.AccountID, identity.Region))
	if err != nil {
		return nil, err
	}

	a.stagingStore = s

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

func (a *App) getParser(service string) (staging.Parser, error) {
	switch service {
	case string(staging.ServiceParam):
		return &staging.ParamStrategy{}, nil
	case string(staging.ServiceSecret):
		return &staging.SecretStrategy{}, nil
	default:
		return nil, errInvalidService
	}
}

// serviceStrategy builds the staging strategy for a service, wrapping a
// provider.Store resolved through the registry. The returned *ParamStrategy /
// *SecretStrategy satisfies every staging strategy interface (Edit, Delete,
// Apply, Diff), so the typed getters below narrow it as needed.
func (a *App) serviceStrategy(service string) (staging.FullStrategy, error) {
	switch service {
	case string(staging.ServiceParam):
		s, err := a.paramStore()
		if err != nil {
			return nil, err
		}

		return staging.NewParamStrategy(s), nil
	case string(staging.ServiceSecret):
		s, err := a.secretStore()
		if err != nil {
			return nil, err
		}

		return staging.NewSecretStrategy(s), nil
	default:
		return nil, errInvalidService
	}
}

// strategyAs resolves the service strategy and narrows it to the requested
// staging strategy interface T. The concrete *ParamStrategy / *SecretStrategy
// satisfy every staging strategy interface, so this succeeds for the Edit,
// Apply and Diff interfaces (which FullStrategy embeds) as well as for
// DeleteStrategy (which it does not embed but the concrete types implement).
// It is a free function because Go methods cannot declare type parameters.
func strategyAs[T any](a *App, service string) (T, error) {
	var zero T

	strategy, err := a.serviceStrategy(service)
	if err != nil {
		return zero, err
	}

	narrowed, ok := any(strategy).(T)
	if !ok {
		return zero, errInvalidService
	}

	return narrowed, nil
}

func (a *App) getEditStrategy(service string) (staging.EditStrategy, error) {
	return strategyAs[staging.EditStrategy](a, service)
}

func (a *App) getDeleteStrategy(service string) (staging.DeleteStrategy, error) {
	return strategyAs[staging.DeleteStrategy](a, service)
}

func (a *App) getApplyStrategy(service string) (staging.ApplyStrategy, error) {
	return strategyAs[staging.ApplyStrategy](a, service)
}

func (a *App) getDiffStrategy(service string) (staging.DiffStrategy, error) {
	return strategyAs[staging.DiffStrategy](a, service)
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
