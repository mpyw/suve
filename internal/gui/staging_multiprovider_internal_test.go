//go:build production || dev

package gui

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
)

// TestApp_stagingScope_NoSTSForResolvableScopes verifies that resolving the
// staging scope needs no AWS STS call for non-AWS providers (nor for an AWS
// scope that already carries account+region). This is the property that lets
// multi-provider staging work without credentials for the wrong cloud — and
// replaces #276's interim "reject non-AWS" guard.
func TestApp_stagingScope_NoSTSForResolvableScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		scope provider.Scope
	}{
		{"google cloud", provider.GoogleCloudScope("proj")},
		{"azure key vault", provider.AzureKeyVaultScope("vault")},
		{"azure app config", provider.AzureAppConfigScope("store")},
		{"aws with account+region", provider.AWSScope("123456789012", "us-east-1")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := &App{scope: tt.scope}

			// No AWS credentials configured in the test env; this must not error,
			// proving no STS round-trip occurred.
			got, err := app.stagingScope()
			require.NoError(t, err)
			assert.Equal(t, tt.scope, got)
			assert.Equal(t, tt.scope.Key(), got.Key())
		})
	}
}

// TestApp_getParser_PerProvider pins the store-less parser selection used by
// status/reset to the active provider + service.
func TestApp_getParser_PerProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider provider.Provider
		service  string
		want     staging.Parser
	}{
		{"aws param", provider.ProviderAWS, "param", &staging.AWSParamStrategy{}},
		{"aws secret", provider.ProviderAWS, "secret", &staging.AWSSecretStrategy{}},
		{"google cloud secret", provider.ProviderGoogleCloud, "secret", &staging.GoogleCloudSecretStrategy{}},
		{"azure param", provider.ProviderAzure, "param", &staging.AzureAppConfigParamStrategy{}},
		{"azure secret", provider.ProviderAzure, "secret", &staging.AzureKeyVaultSecretStrategy{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := &App{scope: provider.Scope{Provider: tt.provider}}

			parser, err := app.getParser(tt.service)
			require.NoError(t, err)
			assert.IsType(t, tt.want, parser)
		})
	}
}

// TestApp_getStagingStore_ScopeKeyed verifies that the working store is keyed by
// scope (each provider/scope gets its own store, cached per key) and — for a
// non-AWS scope — resolves without any AWS STS call.
func TestApp_getStagingStore_ScopeKeyed(t *testing.T) {
	// Non-parallel: sets process env (staging key + HOME).
	t.Setenv("SUVE_STAGING_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("HOME", t.TempDir())

	app := NewApp(provider.Scope{Provider: provider.ProviderGoogleCloud}, "")
	app.Startup(t.Context())
	app.scope = provider.GoogleCloudScope("proj-a")

	s1, err := app.getStagingStore(provider.KindSecret)
	require.NoError(t, err) // Google Cloud: no STS; key via SUVE_STAGING_KEY
	require.NotNil(t, s1)

	// Same scope → cached same instance.
	s1b, err := app.getStagingStore(provider.KindSecret)
	require.NoError(t, err)
	assert.Same(t, s1, s1b, "same scope should return the cached store")

	// Different scope → different, isolated store.
	app.scope = provider.GoogleCloudScope("proj-b")

	s2, err := app.getStagingStore(provider.KindSecret)
	require.NoError(t, err)
	assert.NotSame(t, s1, s2, "a different scope must get its own store")
}

// TestStagingScope_GUICLIParity is the cross-surface guard: for every
// (provider, service) the GUI's staging scope key MUST equal the CLI's, so a
// change staged in one surface is always visible in the other. It is the
// regression test for the App Configuration divergence — the GUI holds a
// COMBINED Azure scope (both vault and store) and scope.Key() resolves such a
// scope to the Key Vault key (VaultName is checked first), which silently keyed
// App Configuration staging under the Key Vault bucket.
//
// The CLI resolvers (internal/cli/commands/internal, not importable here under
// Go's internal rule) are thin wrappers over the provider.*Scope constructors
// asserted below, so those keys ARE the CLI's staging keys:
//   - AzureAppConfigStagingScopeResolver -> provider.AzureAppConfigScope(store)
//   - AzureKeyVaultStagingScopeResolver  -> provider.AzureKeyVaultScope(vault)
//   - GoogleCloudStagingScopeResolver    -> provider.GoogleCloudScope(project)
//   - AWSScopeResolver                   -> provider.AWSScope(account, region)
func TestStagingScope_GUICLIParity(t *testing.T) {
	t.Parallel()

	// The GUI holds a COMBINED Azure scope (both vault and store), which is the
	// exact shape that trapped scope.Key() into the Key Vault bucket for param.
	azure := provider.Scope{
		Provider: provider.ProviderAzure, VaultName: "myvault", StoreName: "mystore", AppConfigNamespace: "dev",
	}

	aws := provider.AWSScope("123456789012", "us-east-1")
	gcloud := provider.GoogleCloudScope("proj")

	tests := []struct {
		name   string
		scope  provider.Scope
		kind   provider.Kind
		cliKey string
	}{
		{"azure param -> App Configuration store", azure, provider.KindParam, provider.AzureAppConfigScope("mystore").Key()},
		{"azure secret -> Key Vault", azure, provider.KindSecret, provider.AzureKeyVaultScope("myvault").Key()},
		{"google cloud secret -> project", gcloud, provider.KindSecret, gcloud.Key()},
		{"aws param -> account", aws, provider.KindParam, aws.Key()},
		{"aws secret -> account", aws, provider.KindSecret, aws.Key()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gui := &App{scope: tt.scope}
			guiScope, err := gui.stagingScopeForKind(tt.kind)
			require.NoError(t, err)

			assert.Equal(t, tt.cliKey, guiScope.Key(),
				"GUI staging key must equal the CLI staging key for %s", tt.name)
		})
	}

	// The two Azure services must NOT share a bucket (the specific bug).
	gui := &App{scope: azure}
	paramScope, err := gui.stagingScopeForKind(provider.KindParam)
	require.NoError(t, err)
	secretScope, err := gui.stagingScopeForKind(provider.KindSecret)
	require.NoError(t, err)
	assert.NotEqual(t, paramScope.Key(), secretScope.Key(),
		"Azure App Configuration and Key Vault staging must not collide")
}
