//go:build production || dev

package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
)

func TestApp_SelectScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sel       ScopeSelection
		wantErr   error
		wantScope provider.Scope
	}{
		{
			name:      "aws (no fields required)",
			sel:       ScopeSelection{Provider: "aws"},
			wantScope: provider.Scope{Provider: provider.ProviderAWS},
		},
		{
			name:      "googlecloud with project",
			sel:       ScopeSelection{Provider: "googlecloud", ProjectID: "my-project"},
			wantScope: provider.GoogleCloudScope("my-project"),
		},
		{
			name:    "googlecloud missing project",
			sel:     ScopeSelection{Provider: "googlecloud"},
			wantErr: errGoogleCloudProjectRequired,
		},
		{
			name: "azure with vault only",
			sel:  ScopeSelection{Provider: "azure", VaultName: "vault"},
			wantScope: provider.Scope{
				Provider: provider.ProviderAzure, VaultName: "vault",
			},
		},
		{
			name: "azure with store only",
			sel:  ScopeSelection{Provider: "azure", StoreName: "store"},
			wantScope: provider.Scope{
				Provider: provider.ProviderAzure, StoreName: "store",
			},
		},
		{
			name: "azure with both vault and store",
			sel:  ScopeSelection{Provider: "azure", VaultName: "vault", StoreName: "store"},
			wantScope: provider.Scope{
				Provider: provider.ProviderAzure, VaultName: "vault", StoreName: "store",
			},
		},
		{
			name:    "azure missing both vault and store",
			sel:     ScopeSelection{Provider: "azure"},
			wantErr: errAzureScopeRequired,
		},
		{
			name:    "unknown provider",
			sel:     ScopeSelection{Provider: "oracle"},
			wantErr: errInvalidProvider,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := NewApp(provider.Scope{Provider: provider.ProviderAWS})

			err := app.SelectScope(tt.sel)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				// A rejected selection must not mutate the active scope.
				assert.Equal(t, provider.Scope{Provider: provider.ProviderAWS}, app.currentScope())

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantScope, app.currentScope())
		})
	}
}

func TestApp_GetCurrentScope_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sel  ScopeSelection
	}{
		{name: "aws", sel: ScopeSelection{Provider: "aws"}},
		{name: "googlecloud", sel: ScopeSelection{Provider: "googlecloud", ProjectID: "proj"}},
		{
			name: "azure key vault + app config",
			sel:  ScopeSelection{Provider: "azure", VaultName: "vault", StoreName: "store"},
		},
		{
			// Only one Azure service configured: Key Vault (secret) available,
			// App Configuration (param) absent. The readback must preserve the
			// empty storeName so #267 can render the half-filled form.
			name: "azure key vault only",
			sel:  ScopeSelection{Provider: "azure", VaultName: "vault"},
		},
		{
			// Mirror case: only App Configuration (param) available, no vault.
			name: "azure app config only",
			sel:  ScopeSelection{Provider: "azure", StoreName: "store"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := NewApp(provider.Scope{Provider: provider.ProviderAWS})
			require.NoError(t, app.SelectScope(tt.sel))

			got := app.GetCurrentScope()
			require.NotNil(t, got)
			// Each input populates only provider-relevant fields, so the
			// readback must equal it verbatim — including the empty side of a
			// one-sided Azure scope (vault-only / store-only).
			assert.Equal(t, &tt.sel, got)
		})
	}
}

// TestApp_GetCurrentScope_EnvDerivedInitialScope verifies that the env-derived
// initial scope (no SelectScope yet) is surfaced for form prefill.
func TestApp_GetCurrentScope_EnvDerivedInitialScope(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")

	app := NewApp(provider.Scope{Provider: provider.ProviderGoogleCloud})

	got := app.GetCurrentScope()
	require.NotNil(t, got)
	assert.Equal(t, string(provider.ProviderGoogleCloud), got.Provider)
	assert.Equal(t, "env-project", got.ProjectID)
}

func TestApp_GetCurrentScope_EnvDerivedAzure(t *testing.T) {
	t.Setenv("AZURE_KEYVAULT_NAME", "env-vault")
	t.Setenv("AZURE_APPCONFIG_NAME", "env-store")

	app := NewApp(provider.Scope{Provider: provider.ProviderAzure})

	got := app.GetCurrentScope()
	require.NotNil(t, got)
	assert.Equal(t, string(provider.ProviderAzure), got.Provider)
	assert.Equal(t, "env-vault", got.VaultName)
	assert.Equal(t, "env-store", got.StoreName)
}

// TestApp_GetCurrentScope_EnvDerivedAzure_VaultOnly covers a one-sided Azure
// environment: only the Key Vault (secret) side is configured, so App
// Configuration (param) stays absent. AZURE_APPCONFIG_NAME is pinned empty so
// the case is hermetic regardless of the ambient environment.
func TestApp_GetCurrentScope_EnvDerivedAzure_VaultOnly(t *testing.T) {
	t.Setenv("AZURE_KEYVAULT_NAME", "env-vault")
	t.Setenv("AZURE_APPCONFIG_NAME", "")

	app := NewApp(provider.Scope{Provider: provider.ProviderAzure})

	got := app.GetCurrentScope()
	require.NotNil(t, got)
	assert.Equal(t, string(provider.ProviderAzure), got.Provider)
	assert.Equal(t, "env-vault", got.VaultName)
	assert.Empty(t, got.StoreName)
}

// TestApp_GetCurrentScope_EnvDerivedAzure_StoreOnly is the mirror: only App
// Configuration (param) is configured; Key Vault stays absent.
func TestApp_GetCurrentScope_EnvDerivedAzure_StoreOnly(t *testing.T) {
	t.Setenv("AZURE_KEYVAULT_NAME", "")
	t.Setenv("AZURE_APPCONFIG_NAME", "env-store")

	app := NewApp(provider.Scope{Provider: provider.ProviderAzure})

	got := app.GetCurrentScope()
	require.NotNil(t, got)
	assert.Equal(t, string(provider.ProviderAzure), got.Provider)
	assert.Empty(t, got.VaultName)
	assert.Equal(t, "env-store", got.StoreName)
}
