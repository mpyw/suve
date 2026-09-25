// Package internal holds the Azure scope context, stores and staging-scope
// wiring that the azure param (App Configuration), secret (Key Vault) and
// stage command packages share.
package internal

import (
	"context"
	"errors"
	"fmt"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
)

// scopeContextKey keys the resolved Azure scope fields stored in the context by
// the azure command group's Before hooks.
type scopeContextKey struct{}

// scopeCtx holds the Azure scope fields resolved from flags/env by the azure
// command group. Each subgroup (secret/param) sets its vault/store name via its
// Before hook.
type scopeCtx struct {
	vaultName          string
	storeName          string
	appConfigNamespace string
}

func scopeFromContext(ctx context.Context) scopeCtx {
	sc, _ := ctx.Value(scopeContextKey{}).(scopeCtx)

	return sc
}

// WithVaultName returns a context carrying the resolved Azure Key Vault name,
// merged onto any base scope already present. The azure secret subgroup's
// Before hook sets it (from --vault-name or its env fallback).
func WithVaultName(ctx context.Context, vaultName string) context.Context {
	sc := scopeFromContext(ctx)
	sc.vaultName = vaultName

	return context.WithValue(ctx, scopeContextKey{}, sc)
}

// WithStoreName returns a context carrying the resolved Azure App Configuration
// store name, merged onto any base scope already present. The azure param
// subgroup's Before hook sets it (from --store-name or its env fallback).
func WithStoreName(ctx context.Context, storeName string) context.Context {
	sc := scopeFromContext(ctx)
	sc.storeName = storeName

	return context.WithValue(ctx, scopeContextKey{}, sc)
}

// WithAppConfigNamespace returns a context carrying the resolved Azure App
// Configuration namespace (the axis Azure calls a "label"), merged onto any base
// scope already present. The azure param subgroup's Before hook sets it (from
// --namespace/--ns or the AZURE_APPCONFIG_NAMESPACE env). Empty is the null
// (default) namespace and also overrides an env default back to null.
func WithAppConfigNamespace(ctx context.Context, namespace string) context.Context {
	sc := scopeFromContext(ctx)
	sc.appConfigNamespace = namespace

	return context.WithValue(ctx, scopeContextKey{}, sc)
}

// AppConfigNamespace returns the Azure App Configuration namespace resolved
// into ctx by WithAppConfigNamespace (empty = the null/default namespace).
func AppConfigNamespace(ctx context.Context) string {
	return scopeFromContext(ctx).appConfigNamespace
}

// appConfigScope builds the App Configuration scope for the context's store
// name and namespace.
func appConfigScope(sc scopeCtx) provider.Scope {
	scope := provider.AzureAppConfigScope(sc.storeName)
	scope.AppConfigNamespace = sc.appConfigNamespace

	return scope
}

// KeyVaultStore resolves a provider.Store for the Azure Key Vault (secret)
// service. The vault name is read from the context (see WithVaultName); it
// returns a clear error when no vault name was resolved.
func KeyVaultStore(ctx context.Context) (provider.Store, error) {
	sc := scopeFromContext(ctx)
	if sc.vaultName == "" {
		return nil, errors.New(
			"no Azure Key Vault specified: set --vault-name or the AZURE_KEYVAULT_NAME environment variable",
		)
	}

	return cliinternal.Store(ctx, provider.AzureKeyVaultScope(sc.vaultName), provider.KindSecret)
}

// AppConfigStore resolves a provider.Store for the Azure App Configuration
// (param) service. The store name is read from the context (see WithStoreName);
// it returns a clear error when no store name was resolved.
func AppConfigStore(ctx context.Context) (provider.Store, error) {
	sc := scopeFromContext(ctx)
	if sc.storeName == "" {
		return nil, errors.New(
			"no Azure App Configuration store specified: set --store-name or the AZURE_APPCONFIG_NAME environment variable",
		)
	}

	return cliinternal.Store(ctx, appConfigScope(sc), provider.KindParam)
}

// KeyVaultConfirmTarget describes the Key Vault for a confirmation prompt
// ("vault my-vault"). It returns "" when no vault was resolved.
func KeyVaultConfirmTarget(ctx context.Context) string {
	sc := scopeFromContext(ctx)
	if sc.vaultName == "" {
		return ""
	}

	return provider.AzureKeyVaultScope(sc.vaultName).Target().String()
}

// AppConfigConfirmTarget describes the App Configuration store and namespace
// for a confirmation prompt. It returns "" when no store was resolved.
func AppConfigConfirmTarget(ctx context.Context) string {
	sc := scopeFromContext(ctx)
	if sc.storeName == "" {
		return ""
	}

	return appConfigScope(sc).Target().String()
}

// KeyVaultStagingScopeResolver resolves the Azure Key Vault staging scope from
// the vault name stashed in the context (see WithVaultName). It performs no
// network calls.
func KeyVaultStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	sc := scopeFromContext(ctx)
	if sc.vaultName == "" {
		return staging.ResolvedScope{}, fmt.Errorf(
			"%w: no Azure Key Vault specified: set --vault-name or the AZURE_KEYVAULT_NAME environment variable",
			staging.ErrServiceNotConfigured,
		)
	}

	return binding.StagingScope(ctx, provider.AzureKeyVaultScope(sc.vaultName), provider.KindSecret, nil)
}

// AppConfigStagingScopeResolver resolves the Azure App Configuration staging
// scope from the store name stashed in the context (see WithStoreName). It
// performs no network calls.
func AppConfigStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	sc := scopeFromContext(ctx)
	if sc.storeName == "" {
		return staging.ResolvedScope{}, fmt.Errorf(
			"%w: no Azure App Configuration store specified: set --store-name or the AZURE_APPCONFIG_NAME environment variable",
			staging.ErrServiceNotConfigured,
		)
	}

	return binding.StagingScope(ctx, appConfigScope(sc), provider.KindParam, nil)
}
