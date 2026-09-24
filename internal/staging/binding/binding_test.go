package binding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
	"github.com/mpyw/suve/internal/version"
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

// TestLookup_ItemNameMatchesCapability pins that every capability service's
// ItemNoun (the TUI and GUI wording) equals its staging strategy's ItemName (the
// CLI wording), so a noun changed in one place cannot drift from the other.
func TestLookup_ItemNameMatchesCapability(t *testing.T) {
	t.Parallel()

	for _, pc := range capability.All() {
		for _, sc := range pc.Services {
			b, err := binding.Lookup(provider.Provider(pc.Provider), provider.Kind(sc.Service))
			require.NoError(t, err, "%s %s", pc.Provider, sc.Service)
			assert.Equal(t, sc.ItemNoun, b.Parser().ItemName(), "%s %s", pc.Provider, sc.Service)
		}
	}
}

// TestBinding_SplitSpec pins each product's grammar: the same input splits
// differently per (provider, kind).
func TestBinding_SplitSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   provider.Provider
		kind       provider.Kind
		input      string
		wantName   string
		wantSuffix string
		wantErr    error
	}{
		{"aws param numeric", provider.ProviderAWS, provider.KindParam, "/p#3~~", "/p", "#3~2", nil},
		{"aws param rejects id", provider.ProviderAWS, provider.KindParam, "/p#abc", "", "", version.ErrInvalidNumericVersion},
		{"aws secret label", provider.ProviderAWS, provider.KindSecret, "s:AWSPREVIOUS~1", "s", ":AWSPREVIOUS~1", nil},
		{"google cloud secret numeric", provider.ProviderGoogleCloud, provider.KindSecret, "s#5~", "s", "#5~1", nil},
		{
			"google cloud secret rejects label", provider.ProviderGoogleCloud, provider.KindSecret, "s:x", "", "",
			version.ErrGoogleCloudSecretManagerLabelUnsupported,
		},
		{"azure param keeps the whole key", provider.ProviderAzure, provider.KindParam, "k#3~1", "k#3~1", "", nil},
		{"azure secret opaque id", provider.ProviderAzure, provider.KindSecret, "s#deadbeef", "s", "#deadbeef", nil},
		{"azure secret rejects label", provider.ProviderAzure, provider.KindSecret, "s:x", "", "", version.ErrAzureKeyVaultLabelUnsupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, err := binding.Lookup(tt.provider, tt.kind)
			require.NoError(t, err)

			name, suffix, err := b.SplitSpec(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantSuffix, suffix)
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
			wantTarget: "store mystore · namespace dev",
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
			wantTarget: "account 123456789012 · region us-east-1",
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
			assert.Equal(t, tt.wantTarget, got.Target.String())
		})
	}
}

func TestStagingScope_AWSUsesLookup(t *testing.T) {
	t.Parallel()

	want := staging.ResolvedScope{Scope: provider.AWSScope("123456789012", "us-east-1"), Target: provider.AWSTarget("dev", "123456789012", "us-east-1")}

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

func TestResolveTarget(t *testing.T) {
	t.Parallel()

	t.Run("a scope that describes itself skips the lookup", func(t *testing.T) {
		t.Parallel()

		lookup := func(context.Context) (staging.ResolvedScope, error) {
			return staging.ResolvedScope{}, errors.New("lookup must not run")
		}

		got, err := binding.ResolveTarget(t.Context(), provider.GoogleCloudScope("proj"), lookup)
		require.NoError(t, err)
		assert.Equal(t, "project proj", got.String())
	})

	t.Run("a pending AWS target runs the lookup", func(t *testing.T) {
		t.Parallel()

		want := provider.AWSTarget("dev", "123456789012", "us-east-1")
		lookup := func(context.Context) (staging.ResolvedScope, error) {
			return staging.ResolvedScope{Scope: provider.AWSScope("123456789012", "us-east-1"), Target: want}, nil
		}

		got, err := binding.ResolveTarget(t.Context(), provider.Scope{Provider: provider.ProviderAWS}, lookup)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a lookup failure is returned", func(t *testing.T) {
		t.Parallel()

		lookup := func(context.Context) (staging.ResolvedScope, error) {
			return staging.ResolvedScope{}, errors.New("no credentials")
		}

		_, err := binding.ResolveTarget(t.Context(), provider.Scope{Provider: provider.ProviderAWS}, lookup)
		require.ErrorContains(t, err, "no credentials")
	})
}
