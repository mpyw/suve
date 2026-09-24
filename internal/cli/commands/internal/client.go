// client.go is this package's subject: the wiring that assembles each
// provider's store, strategy and scope, which every command goes through. Core,
// so that internal.AWSParamStore does not have to become ClientAWSParamStore.
//declscope:core

package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/builtin"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
)

// registry is the provider registry reachable by every CLI command, built by
// builtin.NewRegistry (the composition the GUI and TUI share). Top-level command
// groups build their own provider.Scope and resolve stores through it.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
var registry = builtin.NewRegistry()

// gcloudProjectContextKey keys the resolved Google Cloud project id stored in the
// context by the gcloud command group's Before hook.
type gcloudProjectContextKey struct{}

// WithGoogleCloudProject returns a context carrying the resolved Google Cloud project
// id. The gcloud command group sets it once (from --project or the
// GOOGLE_CLOUD_PROJECT env) so every gcloud subcommand can resolve a store
// without threading the flag through the generic command Config.
func WithGoogleCloudProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, gcloudProjectContextKey{}, project)
}

func gcloudProjectFromContext(ctx context.Context) string {
	project, _ := ctx.Value(gcloudProjectContextKey{}).(string)

	return project
}

// azureScopeContextKey keys the resolved Azure scope fields stored in the
// context by the azure command group's Before hooks.
type azureScopeContextKey struct{}

// azureScopeCtx holds the Azure scope fields resolved from flags/env by the
// azure command group. Each subgroup (secret/param) sets its vault/store name
// via its Before hook.
type azureScopeCtx struct {
	vaultName          string
	storeName          string
	appConfigNamespace string
}

func azureScopeFromContext(ctx context.Context) azureScopeCtx {
	sc, _ := ctx.Value(azureScopeContextKey{}).(azureScopeCtx)

	return sc
}

// WithAzureVaultName returns a context carrying the resolved Azure Key Vault
// name, merged onto any base scope already present. The azure secret subgroup's
// Before hook sets it (from --vault-name or its env fallback).
func WithAzureVaultName(ctx context.Context, vaultName string) context.Context {
	sc := azureScopeFromContext(ctx)
	sc.vaultName = vaultName

	return context.WithValue(ctx, azureScopeContextKey{}, sc)
}

// WithAzureStoreName returns a context carrying the resolved Azure App
// Configuration store name, merged onto any base scope already present. The
// azure param subgroup's Before hook sets it (from --store-name or its env
// fallback).
func WithAzureStoreName(ctx context.Context, storeName string) context.Context {
	sc := azureScopeFromContext(ctx)
	sc.storeName = storeName

	return context.WithValue(ctx, azureScopeContextKey{}, sc)
}

// WithAzureAppConfigNamespace returns a context carrying the resolved Azure App
// Configuration namespace (the axis Azure calls a "label"), merged onto any base
// scope already present. The azure param subgroup's Before hook sets it (from
// --namespace/--ns or the AZURE_APPCONFIG_NAMESPACE env). Empty is the null
// (default) namespace and also overrides an env default back to null.
func WithAzureAppConfigNamespace(ctx context.Context, namespace string) context.Context {
	sc := azureScopeFromContext(ctx)
	sc.appConfigNamespace = namespace

	return context.WithValue(ctx, azureScopeContextKey{}, sc)
}

// AzureAppConfigNamespace returns the Azure App Configuration namespace resolved
// into ctx by WithAzureAppConfigNamespace (empty = the null/default namespace).
func AzureAppConfigNamespace(ctx context.Context) string {
	return azureScopeFromContext(ctx).appConfigNamespace
}

// AWSParamStore resolves a provider.Store for AWS SSM Parameter Store via the
// registry. The AWS factory builds its client from the ambient AWS config
// (region from env/profile), so the scope carries only the provider.
func AWSParamStore(ctx context.Context) (provider.Store, error) {
	return registry.Store(ctx, provider.Scope{Provider: provider.ProviderAWS}, provider.KindParam)
}

// AWSSecretStore resolves a provider.Store for AWS Secrets Manager via the
// registry. The AWS factory builds its client from the ambient AWS config
// (region from env/profile), so the scope carries only the provider.
func AWSSecretStore(ctx context.Context) (provider.Store, error) {
	return registry.Store(ctx, provider.Scope{Provider: provider.ProviderAWS}, provider.KindSecret)
}

// GoogleCloudSecretStore resolves a provider.Store for the Google Cloud Secret Manager
// service. The project id is read from the context (see WithGoogleCloudProject); it
// returns a clear error when no project could be resolved.
func GoogleCloudSecretStore(ctx context.Context) (provider.Store, error) {
	project := gcloudProjectFromContext(ctx)
	if project == "" {
		return nil, errors.New(
			"no Google Cloud project specified: set --project or the GOOGLE_CLOUD_PROJECT environment variable",
		)
	}

	return registry.Store(ctx, provider.GoogleCloudScope(project), provider.KindSecret)
}

// AzureKeyVaultStore resolves a provider.Store for the Azure Key Vault (secret)
// service. The vault name is read from the context (see WithAzureVaultName); it
// returns a clear error when no vault name was resolved.
func AzureKeyVaultStore(ctx context.Context) (provider.Store, error) {
	sc := azureScopeFromContext(ctx)
	if sc.vaultName == "" {
		return nil, errors.New(
			"no Azure Key Vault specified: set --vault-name or the AZURE_KEYVAULT_NAME environment variable",
		)
	}

	scope := provider.AzureKeyVaultScope(sc.vaultName)

	return registry.Store(ctx, scope, provider.KindSecret)
}

// AzureAppConfigStore resolves a provider.Store for the Azure App Configuration
// (param) service. The store name is read from the context (see
// WithAzureStoreName); it returns a clear error when no store name was resolved.
func AzureAppConfigStore(ctx context.Context) (provider.Store, error) {
	sc := azureScopeFromContext(ctx)
	if sc.storeName == "" {
		return nil, errors.New(
			"no Azure App Configuration store specified: set --store-name or the AZURE_APPCONFIG_NAME environment variable",
		)
	}

	scope := provider.AzureAppConfigScope(sc.storeName)
	scope.AppConfigNamespace = sc.appConfigNamespace

	return registry.Store(ctx, scope, provider.KindParam)
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

// AWSStagingScopeResolver resolves the AWS staging scope (account + region)
// from the STS caller identity, through the shared staging binding. Both AWS
// services share it. It satisfies staging.ScopeResolver.
func AWSStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	return binding.StagingScope(ctx, provider.Scope{Provider: provider.ProviderAWS}, provider.KindParam, nil)
}

// GoogleCloudStagingScopeResolver resolves the Google Cloud staging scope from the
// project stashed in the context (see WithGoogleCloudProject). It performs no network
// calls. It satisfies staging.ScopeResolver.
func GoogleCloudStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	project := gcloudProjectFromContext(ctx)
	if project == "" {
		return staging.ResolvedScope{}, errors.New(
			"no Google Cloud project specified: set --project or the GOOGLE_CLOUD_PROJECT environment variable",
		)
	}

	return binding.StagingScope(ctx, provider.GoogleCloudScope(project), provider.KindSecret, nil)
}

// AzureKeyVaultStagingScopeResolver resolves the Azure Key Vault staging scope
// from the vault name stashed in the context (see WithAzureVaultName). It
// performs no network calls.
func AzureKeyVaultStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	sc := azureScopeFromContext(ctx)
	if sc.vaultName == "" {
		return staging.ResolvedScope{}, fmt.Errorf(
			"%w: no Azure Key Vault specified: set --vault-name or the AZURE_KEYVAULT_NAME environment variable",
			staging.ErrServiceNotConfigured,
		)
	}

	return binding.StagingScope(ctx, provider.AzureKeyVaultScope(sc.vaultName), provider.KindSecret, nil)
}

// AzureAppConfigStagingScopeResolver resolves the Azure App Configuration
// staging scope from the store name stashed in the context (see
// WithAzureStoreName). It performs no network calls.
func AzureAppConfigStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	sc := azureScopeFromContext(ctx)
	if sc.storeName == "" {
		return staging.ResolvedScope{}, fmt.Errorf(
			"%w: no Azure App Configuration store specified: set --store-name or the AZURE_APPCONFIG_NAME environment variable",
			staging.ErrServiceNotConfigured,
		)
	}

	scope := provider.AzureAppConfigScope(sc.storeName)
	scope.AppConfigNamespace = sc.appConfigNamespace

	return binding.StagingScope(ctx, scope, provider.KindParam, nil)
}
