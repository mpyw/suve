//go:build production || dev

package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
)

func TestStringError_Error(t *testing.T) {
	t.Parallel()

	err := stringError("test error message")
	assert.Equal(t, "test error message", err.Error())
}

func TestErrInvalidService(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "invalid service: must be 'param' or 'secret'", errInvalidService.Error())
}

// newTestApp builds an App for a known launch scope, failing the test if NewApp
// rejects it.
func newTestApp(t *testing.T, initial provider.Scope, service string) *App {
	t.Helper()

	app, err := NewApp(initial, service)
	require.NoError(t, err)

	return app
}

func TestNewApp(t *testing.T) {
	t.Parallel()

	app := newTestApp(t, provider.Scope{Provider: provider.ProviderAWS}, "")
	assert.NotNil(t, app)
	// Verify the staging store is nil (lazy initialization).
	assert.Nil(t, app.stagingStore)
}

func TestNewApp_NoProviderLeavesScopeUnselected(t *testing.T) {
	t.Parallel()

	// A bare launch with no provider must not silently pick one.
	app := newTestApp(t, provider.Scope{}, "")
	assert.Equal(t, provider.Scope{}, app.currentScope())
	assert.Empty(t, app.InitialProvider())
}

func TestNewApp_UnknownProviderFails(t *testing.T) {
	t.Parallel()

	app, err := NewApp(provider.Scope{Provider: provider.Provider("oracle")}, "")
	require.ErrorIs(t, err, errInvalidProvider)
	assert.Nil(t, app)
}

func TestNewApp_InitialService(t *testing.T) {
	t.Parallel()

	// The launched service is surfaced verbatim via InitialService.
	assert.Equal(t, "param",
		newTestApp(t, provider.Scope{Provider: provider.ProviderAzure}, "param").InitialService())
	assert.Equal(t, "secret",
		newTestApp(t, provider.Scope{Provider: provider.ProviderAzure}, "secret").InitialService())
	// A group-level / bare launch carries no specific service.
	assert.Empty(t,
		newTestApp(t, provider.Scope{Provider: provider.ProviderAzure}, "").InitialService())
}

func TestHydrateScope_FlagWinsElseEnv(t *testing.T) {
	// Not parallel: mutates process env via t.Setenv.
	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")
	t.Setenv("AZURE_KEYVAULT_NAME", "env-vault")
	t.Setenv("AZURE_APPCONFIG_NAME", "env-store")
	t.Setenv("AZURE_APPCONFIG_NAMESPACE", "env-ns")

	// Flag-supplied values win over the environment.
	gc := mustHydrate(t, provider.Scope{Provider: provider.ProviderGoogleCloud, ProjectID: "flag-project"})
	assert.Equal(t, "flag-project", gc.ProjectID)

	az := mustHydrate(t, provider.Scope{Provider: provider.ProviderAzure, VaultName: "flag-vault", AppConfigNamespace: "flag-ns"})
	assert.Equal(t, "flag-vault", az.VaultName)
	// The namespace flag wins over AZURE_APPCONFIG_NAMESPACE.
	assert.Equal(t, "flag-ns", az.AppConfigNamespace)
	// The unset side still falls back to env.
	assert.Equal(t, "env-store", az.StoreName)

	// Empty scopes fall back to env entirely.
	assert.Equal(t, "env-project", mustHydrate(t, provider.Scope{Provider: provider.ProviderGoogleCloud}).ProjectID)
	azEnv := mustHydrate(t, provider.Scope{Provider: provider.ProviderAzure})
	assert.Equal(t, "env-vault", azEnv.VaultName)
	assert.Equal(t, "env-store", azEnv.StoreName)
	// The namespace falls back to AZURE_APPCONFIG_NAMESPACE when the flag is unset.
	assert.Equal(t, "env-ns", azEnv.AppConfigNamespace)

	// AWS carries no resource field (region from ambient config).
	assert.Equal(t, provider.Scope{Provider: provider.ProviderAWS}, mustHydrate(t, provider.Scope{Provider: provider.ProviderAWS}))

	// An unknown or empty provider is an error, never a silent default.
	for _, p := range []provider.Provider{"", "oracle"} {
		_, err := hydrateScope(provider.Scope{Provider: p})
		require.ErrorIs(t, err, errInvalidProvider, "provider %q", p)
	}
}

// mustHydrate runs hydrateScope for a known provider, failing the test on error.
func mustHydrate(t *testing.T, s provider.Scope) provider.Scope {
	t.Helper()

	got, err := hydrateScope(s)
	require.NoError(t, err)

	return got
}

func TestApp_Startup(t *testing.T) {
	t.Parallel()

	app := newTestApp(t, provider.Scope{Provider: provider.ProviderAWS}, "")
	assert.Nil(t, app.ctx)

	app.Startup(t.Context())
	assert.Equal(t, t.Context(), app.ctx)
}

func TestApp_getService_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		service     string
		expectError bool
	}{
		{
			name:        "uppercase PARAM",
			service:     "PARAM",
			expectError: true,
		},
		{
			name:        "mixed case Secret",
			service:     "Secret",
			expectError: true,
		},
		{
			name:        "with whitespace",
			service:     " param",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := &App{}

			_, err := app.getService(tt.service)
			if tt.expectError {
				assert.ErrorIs(t, err, errInvalidService)
			}
		})
	}
}

func TestApp_getParser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		service     string
		expectError bool
	}{
		{
			name:        "param parser",
			service:     "param",
			expectError: false,
		},
		{
			name:        "secret parser",
			service:     "secret",
			expectError: false,
		},
		{
			name:        "invalid service",
			service:     "invalid",
			expectError: true,
		},
		{
			name:        "empty service",
			service:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := &App{}

			parser, err := app.getParserScoped(provider.Scope{Provider: provider.ProviderAWS}, tt.service)
			if tt.expectError {
				require.ErrorIs(t, err, errInvalidService)
				assert.Nil(t, parser)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, parser)
			}
		})
	}
}
