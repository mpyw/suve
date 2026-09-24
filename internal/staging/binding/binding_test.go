package binding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
)

func TestLookup_PerProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider provider.Provider
		kind     provider.Kind
		want     staging.FullStrategy
	}{
		{"aws param", provider.ProviderAWS, provider.KindParam, &staging.AWSParamStrategy{}},
		{"aws secret", provider.ProviderAWS, provider.KindSecret, &staging.AWSSecretStrategy{}},
		{"google cloud secret", provider.ProviderGoogleCloud, provider.KindSecret, &staging.GoogleCloudSecretStrategy{}},
		{"azure param", provider.ProviderAzure, provider.KindParam, &staging.AzureAppConfigParamStrategy{}},
		{"azure secret", provider.ProviderAzure, provider.KindSecret, &staging.AzureKeyVaultSecretStrategy{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, err := binding.Lookup(tt.provider, tt.kind)
			require.NoError(t, err)
			assert.IsType(t, tt.want, b.Strategy(nil))
			assert.IsType(t, tt.want, b.Parser())
			assert.IsType(t, tt.want, b.ParserFactory()())
		})
	}
}

func TestLookup_Errors(t *testing.T) {
	t.Parallel()

	for _, p := range []provider.Provider{"", "oracle"} {
		_, err := binding.Lookup(p, provider.KindSecret)
		require.ErrorIs(t, err, binding.ErrUnknownProvider, "provider %q", p)
	}

	// Google Cloud offers no param service.
	_, err := binding.Lookup(provider.ProviderGoogleCloud, provider.KindParam)
	require.ErrorIs(t, err, provider.ErrUnsupportedKind)
	require.NotErrorIs(t, err, binding.ErrUnknownProvider)
}

func TestBinding_Namespace(t *testing.T) {
	t.Parallel()

	appConfig, err := binding.Lookup(provider.ProviderAzure, provider.KindParam)
	require.NoError(t, err)

	keyVault, err := binding.Lookup(provider.ProviderAzure, provider.KindSecret)
	require.NoError(t, err)

	awsParam, err := binding.Lookup(provider.ProviderAWS, provider.KindParam)
	require.NoError(t, err)

	store := provider.Scope{Provider: provider.ProviderAzure, StoreName: "s", AppConfigNamespace: "base"}

	assert.True(t, appConfig.Namespaced(store))
	assert.Equal(t, "dev", appConfig.NamespaceScope(store, "dev").AppConfigNamespace)
	assert.Equal(t, "base", store.AppConfigNamespace, "the input scope is not mutated")

	// No store name means no App Configuration service to namespace.
	vaultOnly := provider.AzureKeyVaultScope("v")
	assert.False(t, appConfig.Namespaced(vaultOnly))
	assert.Equal(t, vaultOnly, appConfig.NamespaceScope(vaultOnly, "dev"))

	// Services without a namespace axis leave the scope alone.
	assert.False(t, keyVault.Namespaced(store))
	assert.Equal(t, store, keyVault.NamespaceScope(store, "dev"))
	assert.Equal(t, provider.Scope{Provider: provider.ProviderAWS}, awsParam.NamespaceScope(provider.Scope{Provider: provider.ProviderAWS}, "dev"))
}

func TestStagingScope_NoLookupNeeded(t *testing.T) {
	t.Parallel()

	azure := provider.Scope{
		Provider: provider.ProviderAzure, VaultName: "myvault", StoreName: "mystore", AppConfigNamespace: "dev",
	}

	tests := []struct {
		name       string
		scope      provider.Scope
		kind       provider.Kind
		wantScope  provider.Scope
		wantTarget string
	}{
		{
			name:       "google cloud keys by project",
			scope:      provider.GoogleCloudScope("proj"),
			kind:       provider.KindSecret,
			wantScope:  provider.GoogleCloudScope("proj"),
			wantTarget: "project proj",
		},
		{
			name:  "azure param keys by App Configuration store",
			scope: azure,
			kind:  provider.KindParam,
			wantScope: provider.Scope{
				Provider: provider.ProviderAzure, StoreName: "mystore", AppConfigNamespace: "dev",
			},
			wantTarget: "store mystore (namespace dev)",
		},
		{
			name:       "azure param without namespace",
			scope:      provider.AzureAppConfigScope("mystore"),
			kind:       provider.KindParam,
			wantScope:  provider.AzureAppConfigScope("mystore"),
			wantTarget: "store mystore",
		},
		{
			name:       "azure secret keys by Key Vault",
			scope:      azure,
			kind:       provider.KindSecret,
			wantScope:  provider.AzureKeyVaultScope("myvault"),
			wantTarget: "vault myvault",
		},
		{
			name:       "aws with account and region",
			scope:      provider.AWSScope("123456789012", "us-east-1"),
			kind:       provider.KindSecret,
			wantScope:  provider.AWSScope("123456789012", "us-east-1"),
			wantTarget: "123456789012 / us-east-1",
		},
	}

	failLookup := func(context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{}, errors.New("lookup must not run")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := binding.StagingScope(t.Context(), tt.scope, tt.kind, failLookup)
			require.NoError(t, err)
			assert.Equal(t, tt.wantScope, got.Scope)
			assert.Equal(t, tt.wantTarget, got.Target)
		})
	}
}

func TestStagingScope_AWSUsesLookup(t *testing.T) {
	t.Parallel()

	want := staging.ResolvedScope{Scope: provider.AWSScope("123456789012", "us-east-1"), Target: "dev (123456789012 / us-east-1)"}

	var calls int

	lookup := func(context.Context) (staging.ResolvedScope, error) {
		calls++

		return want, nil
	}

	for _, kind := range []provider.Kind{provider.KindParam, provider.KindSecret} {
		got, err := binding.StagingScope(t.Context(), provider.Scope{Provider: provider.ProviderAWS}, kind, lookup)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	assert.Equal(t, 2, calls)
}

func TestStagingScope_UnknownProvider(t *testing.T) {
	t.Parallel()

	for _, p := range []provider.Provider{"", "oracle"} {
		_, err := binding.StagingScope(t.Context(), provider.Scope{Provider: p}, provider.KindParam, nil)
		require.ErrorIs(t, err, binding.ErrUnknownProvider, "provider %q", p)

		_, err = binding.DefaultIdentity(p)(t.Context())
		require.ErrorIs(t, err, binding.ErrUnknownProvider, "provider %q", p)
	}

	// Google Cloud keys staging from its project and has no identity lookup.
	_, err := binding.DefaultIdentity(provider.ProviderGoogleCloud)(t.Context())
	require.Error(t, err)
}
