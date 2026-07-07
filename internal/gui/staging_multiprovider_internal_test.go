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
		{"aws param", provider.ProviderAWS, "param", &staging.ParamStrategy{}},
		{"aws secret", provider.ProviderAWS, "secret", &staging.SecretStrategy{}},
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

	app := NewApp(provider.Scope{Provider: provider.ProviderGoogleCloud})
	app.Startup(t.Context())
	app.scope = provider.GoogleCloudScope("proj-a")

	s1, err := app.getStagingStore()
	require.NoError(t, err) // Google Cloud: no STS; key via SUVE_STAGING_KEY
	require.NotNil(t, s1)

	// Same scope → cached same instance.
	s1b, err := app.getStagingStore()
	require.NoError(t, err)
	assert.Same(t, s1, s1b, "same scope should return the cached store")

	// Different scope → different, isolated store.
	app.scope = provider.GoogleCloudScope("proj-b")

	s2, err := app.getStagingStore()
	require.NoError(t, err)
	assert.NotSame(t, s1, s2, "a different scope must get its own store")
}
