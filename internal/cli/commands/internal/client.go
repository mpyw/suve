package internal

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/provider/azure"
	"github.com/mpyw/suve/internal/provider/gcloud"
	"github.com/mpyw/suve/internal/staging"
)

// registry is the provider registry reachable by every CLI command. It is the
// single composition point where cloud backends are wired in: AWS (param +
// secret), Google Cloud (secret only), and Azure (Key Vault secret + App
// Configuration param) are registered here. Top-level command groups build their
// own provider.Scope and resolve stores through this same registry.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
var registry = func() *provider.Registry {
	reg := aws.NewRegistry()
	gcloud.Register(reg)
	azure.Register(reg)

	return reg
}()

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
// azure command group. It is assembled across two Before hooks: the top-level
// azure command sets subscription/resource-group; each subgroup (secret/param)
// sets its vault/store name.
type azureScopeCtx struct {
	subscription  string
	resourceGroup string
	vaultName     string
	storeName     string
}

func azureScopeFromContext(ctx context.Context) azureScopeCtx {
	sc, _ := ctx.Value(azureScopeContextKey{}).(azureScopeCtx)

	return sc
}

// WithAzureBase returns a context carrying the resolved Azure subscription id and
// resource group. The azure command group's Before hook sets it once (from
// --subscription/--resource-group or their env fallbacks).
func WithAzureBase(ctx context.Context, subscription, resourceGroup string) context.Context {
	sc := azureScopeFromContext(ctx)
	sc.subscription = subscription
	sc.resourceGroup = resourceGroup

	return context.WithValue(ctx, azureScopeContextKey{}, sc)
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

// storeScope is the provider selector for read/write commands. Only the
// Provider field is needed: the AWS factory builds its SSM/Secrets Manager
// client from the ambient AWS config (region from env/profile), so no
// account/region lookup — and therefore no STS GetCallerIdentity call — is
// required here. (Account/region only matter for staging-state file keying,
// which the staging commands build from the AWS identity separately.)
//
//nolint:gochecknoglobals // immutable provider selector for read/write commands
var storeScope = provider.Scope{Provider: provider.ProviderAWS}

// ParamStore resolves a provider.Store for the parameter service via the
// registry (AWS by default).
func ParamStore(ctx context.Context) (provider.Store, error) {
	return storeForKind(ctx, provider.KindParam)
}

// SecretStore resolves a provider.Store for the secret service via the
// registry (AWS by default).
func SecretStore(ctx context.Context) (provider.Store, error) {
	return storeForKind(ctx, provider.KindSecret)
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
// service. The scope fields are read from the context (see WithAzureBase /
// WithAzureVaultName); it returns a clear error when no vault name was resolved.
func AzureKeyVaultStore(ctx context.Context) (provider.Store, error) {
	sc := azureScopeFromContext(ctx)
	if sc.vaultName == "" {
		return nil, errors.New(
			"no Azure Key Vault specified: set --vault-name or the AZURE_KEYVAULT_NAME environment variable",
		)
	}

	scope := provider.AzureKeyVaultScope(sc.subscription, sc.resourceGroup, sc.vaultName)

	return registry.Store(ctx, scope, provider.KindSecret)
}

// AzureAppConfigStore resolves a provider.Store for the Azure App Configuration
// (param) service. The scope fields are read from the context (see
// WithAzureBase / WithAzureStoreName); it returns a clear error when no store
// name was resolved.
func AzureAppConfigStore(ctx context.Context) (provider.Store, error) {
	sc := azureScopeFromContext(ctx)
	if sc.storeName == "" {
		return nil, errors.New(
			"no Azure App Configuration store specified: set --store-name or the AZURE_APPCONFIG_NAME environment variable",
		)
	}

	scope := provider.AzureAppConfigScope(sc.subscription, sc.resourceGroup, sc.storeName)

	return registry.Store(ctx, scope, provider.KindParam)
}

func storeForKind(ctx context.Context, kind provider.Kind) (provider.Store, error) {
	store, err := registry.Store(ctx, storeScope, kind)
	if err != nil {
		return nil, err
	}

	return store, nil
}

// ParamStrategyFactory builds a staging FullStrategy for the parameter service,
// wrapping a provider.Store resolved through the registry. It satisfies
// staging.StrategyFactory.
func ParamStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := ParamStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewParamStrategy(store), nil
}

// SecretStrategyFactory builds a staging FullStrategy for the secret service,
// wrapping a provider.Store resolved through the registry. It satisfies
// staging.StrategyFactory.
func SecretStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := SecretStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewSecretStrategy(store), nil
}

// GoogleCloudSecretStrategyFactory builds a staging FullStrategy for Google Cloud Secret
// Manager, wrapping a provider.Store resolved for the context's project. It
// satisfies staging.StrategyFactory.
func GoogleCloudSecretStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := GoogleCloudSecretStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewGoogleCloudSecretStrategy(store), nil
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

	return staging.ResolvedScope{
		Scope:  provider.GoogleCloudScope(project),
		Target: "project " + project,
	}, nil
}

// AzureKeyVaultSecretStrategyFactory builds a staging FullStrategy for Azure Key
// Vault secrets, wrapping a provider.Store resolved for the context's vault. It
// satisfies staging.StrategyFactory.
func AzureKeyVaultSecretStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := AzureKeyVaultStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewAzureKeyVaultSecretStrategy(store), nil
}

// AzureAppConfigParamStrategyFactory builds a staging FullStrategy for Azure App
// Configuration, wrapping a provider.Store resolved for the context's store. It
// satisfies staging.StrategyFactory.
func AzureAppConfigParamStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := AzureAppConfigStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewAzureAppConfigParamStrategy(store), nil
}

// AzureKeyVaultStagingScopeResolver resolves the Azure Key Vault staging scope
// from the subscription / resource group / vault stashed in the context (see
// WithAzureBase / WithAzureVaultName). It performs no network calls.
func AzureKeyVaultStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	sc := azureScopeFromContext(ctx)
	if sc.vaultName == "" {
		return staging.ResolvedScope{}, errors.New(
			"no Azure Key Vault specified: set --vault-name or the AZURE_KEYVAULT_NAME environment variable",
		)
	}

	return staging.ResolvedScope{
		Scope:  provider.AzureKeyVaultScope(sc.subscription, sc.resourceGroup, sc.vaultName),
		Target: "vault " + sc.vaultName,
	}, nil
}

// AzureAppConfigStagingScopeResolver resolves the Azure App Configuration
// staging scope from the subscription / resource group / store stashed in the
// context (see WithAzureBase / WithAzureStoreName). It performs no network calls.
func AzureAppConfigStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	sc := azureScopeFromContext(ctx)
	if sc.storeName == "" {
		return staging.ResolvedScope{}, errors.New(
			"no Azure App Configuration store specified: set --store-name or the AZURE_APPCONFIG_NAME environment variable",
		)
	}

	return staging.ResolvedScope{
		Scope:  provider.AzureAppConfigScope(sc.subscription, sc.resourceGroup, sc.storeName),
		Target: "store " + sc.storeName,
	}, nil
}
