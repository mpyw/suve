// These are client.go's tests, so they share its core namespace.
//declscope:core

package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
)

func TestGoogleCloudStagingScopeResolver(t *testing.T) {
	t.Parallel()

	_, err := GoogleCloudStagingScopeResolver(t.Context())
	require.ErrorContains(t, err, "--project")

	got, err := GoogleCloudStagingScopeResolver(WithGoogleCloudProject(t.Context(), "proj"))
	require.NoError(t, err)
	assert.Equal(t, staging.ResolvedScope{Scope: provider.GoogleCloudScope("proj"), Target: "project proj"}, got)
}

func TestAzureKeyVaultStagingScopeResolver(t *testing.T) {
	t.Parallel()

	_, err := AzureKeyVaultStagingScopeResolver(t.Context())
	require.ErrorIs(t, err, staging.ErrServiceNotConfigured)

	// A store name on the context does not leak into the Key Vault bucket.
	ctx := WithAzureStoreName(WithAzureVaultName(t.Context(), "my-vault"), "my-store")
	got, err := AzureKeyVaultStagingScopeResolver(ctx)
	require.NoError(t, err)
	assert.Equal(t, staging.ResolvedScope{Scope: provider.AzureKeyVaultScope("my-vault"), Target: "vault my-vault"}, got)
}

func TestAzureAppConfigStagingScopeResolver(t *testing.T) {
	t.Parallel()

	_, err := AzureAppConfigStagingScopeResolver(t.Context())
	require.ErrorIs(t, err, staging.ErrServiceNotConfigured)

	ctx := WithAzureAppConfigNamespace(WithAzureStoreName(WithAzureVaultName(t.Context(), "my-vault"), "my-store"), "dev")
	got, err := AzureAppConfigStagingScopeResolver(ctx)
	require.NoError(t, err)

	want := provider.AzureAppConfigScope("my-store")
	want.AppConfigNamespace = "dev"
	assert.Equal(t, staging.ResolvedScope{Scope: want, Target: "store my-store (namespace dev)"}, got)
	assert.Equal(t, "dev", AzureAppConfigNamespace(ctx))
}

func TestStrategyFactory(t *testing.T) {
	t.Parallel()

	factory := StrategyFactory(provider.ProviderGoogleCloud, provider.KindSecret,
		func(context.Context) (provider.Store, error) { return &providermock.Store{}, nil })
	strategy, err := factory(t.Context())
	require.NoError(t, err)
	assert.IsType(t, &staging.GoogleCloudSecretStrategy{}, strategy)

	errStore := errors.New("no store")
	failing := StrategyFactory(provider.ProviderAWS, provider.KindParam,
		func(context.Context) (provider.Store, error) { return nil, errStore })
	_, err = failing(t.Context())
	require.ErrorIs(t, err, errStore)

	assert.IsType(t, &staging.AzureAppConfigParamStrategy{}, ParserFactory(provider.ProviderAzure, provider.KindParam)())
}

func TestStrategyFactory_PanicsWithoutBinding(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { ParserFactory(provider.ProviderGoogleCloud, provider.KindParam) })
	assert.Panics(t, func() { StrategyFactory(provider.Provider("oracle"), provider.KindSecret, nil) })
}
