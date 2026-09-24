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
	"github.com/mpyw/suve/internal/provider/aws/infra"
	"github.com/mpyw/suve/internal/provider/builtin"
	"github.com/mpyw/suve/internal/staging"
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

// AWSParamStrategyFactory builds a staging FullStrategy for the parameter service,
// wrapping a provider.Store resolved through the registry. It satisfies
// staging.StrategyFactory.
func AWSParamStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := AWSParamStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewAWSParamStrategy(store), nil
}

// AWSSecretStrategyFactory builds a staging FullStrategy for the secret service,
// wrapping a provider.Store resolved through the registry. It satisfies
// staging.StrategyFactory.
func AWSSecretStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := AWSSecretStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewAWSSecretStrategy(store), nil
}

// AWSStagingScopeResolver resolves the AWS staging scope (account + region)
// from the STS caller identity. It satisfies staging.ScopeResolver.
func AWSStagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	identity, err := infra.GetAWSIdentity(ctx)
	if err != nil {
		return staging.ResolvedScope{}, fmt.Errorf("failed to get AWS identity: %w", err)
	}

	return staging.ResolvedScope{
		Scope:  provider.AWSScope(identity.AccountID, identity.Region),
		Target: awsStagingTarget(identity.Profile, identity.AccountID, identity.Region),
	}, nil
}

// awsStagingTarget formats the AWS confirmation target line:
// "profile (account / region)" or "account / region".
func awsStagingTarget(profile, accountID, region string) string {
	if accountID == "" || region == "" {
		return ""
	}

	if profile != "" {
		return fmt.Sprintf("%s (%s / %s)", profile, accountID, region)
	}

	return fmt.Sprintf("%s / %s", accountID, region)
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

	return staging.ResolvedScope{
		Scope:  provider.AzureKeyVaultScope(sc.vaultName),
		Target: "vault " + sc.vaultName,
	}, nil
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

	target := "store " + sc.storeName
	if sc.appConfigNamespace != "" {
		target += " (namespace " + sc.appConfigNamespace + ")"
	}

	return staging.ResolvedScope{
		Scope:  scope,
		Target: target,
	}, nil
}
