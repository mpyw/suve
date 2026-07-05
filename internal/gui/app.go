//go:build production || dev

// Package gui provides the Wails-based GUI application.
package gui

import (
	"context"
	"sync"

	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// =============================================================================
// Provider Registry
// =============================================================================

// registry is the provider registry backing the GUI's read/write operations.
// It mirrors the CLI composition point (internal/cli/commands/internal): today
// only AWS is registered, so AWS is the default (and only) provider.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
var registry = aws.NewRegistry()

// storeScope is the provider selector for GUI read/write operations. Only the
// Provider field is needed: the AWS factory builds its SSM/Secrets Manager
// client from the ambient AWS config (region from env/profile), so no
// account/region lookup — and therefore no STS GetCallerIdentity call — is
// required to construct a store. (Account/region only matter for staging-state
// file keying, which getStagingStore derives from the AWS identity separately.)
//
//nolint:gochecknoglobals // immutable provider selector for read/write operations
var storeScope = provider.Scope{Provider: provider.ProviderAWS}

// =============================================================================
// App Struct
// =============================================================================

// App struct holds application state and dependencies.
//
//nolint:containedctx // Wails apps require storing context from Startup
type App struct {
	ctx context.Context

	// Staging store (the working staging area, backed by stage.json)
	stagingStore   store.ReadWriteOperator
	stagingStoreMu sync.Mutex // protects stagingStore initialization
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
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
	return registry.Store(a.ctx, storeScope, provider.KindParam)
}

// secretStore resolves a provider.Store for the secret service via the
// registry (AWS by default).
func (a *App) secretStore() (provider.Store, error) {
	return registry.Store(a.ctx, storeScope, provider.KindSecret)
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
