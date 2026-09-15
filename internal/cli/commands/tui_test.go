//nolint:testpackage // white-box: exercises unexported uniqueTUIProvider/activeTUIProviders/hydrate/validate helpers
package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

// TestUniqueTUIProvider_Unique pins that a single active provider across the
// union of service axes resolves without error, for each provider.
func TestUniqueTUIProvider_Unique(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		det  detect.Result
		want provider.Provider
	}{
		{
			name: "aws only",
			det: detect.Result{
				ParamActive:  []provider.Provider{provider.ProviderAWS},
				SecretActive: []provider.Provider{provider.ProviderAWS},
				StageActive:  []provider.Provider{provider.ProviderAWS},
			},
			want: provider.ProviderAWS,
		},
		{
			name: "google cloud secret only",
			det: detect.Result{
				SecretActive: []provider.Provider{provider.ProviderGoogleCloud},
				StageActive:  []provider.Provider{provider.ProviderGoogleCloud},
			},
			want: provider.ProviderGoogleCloud,
		},
		{
			// Azure param + secret are the same provider, so the union is still one.
			name: "azure param and secret",
			det: detect.Result{
				ParamActive:  []provider.Provider{provider.ProviderAzure},
				SecretActive: []provider.Provider{provider.ProviderAzure},
				StageActive:  []provider.Provider{provider.ProviderAzure},
			},
			want: provider.ProviderAzure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := uniqueTUIProvider(tt.det)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestUniqueTUIProvider_Ambiguous pins the multi-provider error: it names the
// active candidates by provider id and lists the explicit group commands.
func TestUniqueTUIProvider_Ambiguous(t *testing.T) {
	t.Parallel()

	det := detect.Result{
		ParamActive:  []provider.Provider{provider.ProviderAWS},
		SecretActive: []provider.Provider{provider.ProviderAWS, provider.ProviderGoogleCloud},
		StageActive:  []provider.Provider{provider.ProviderAWS, provider.ProviderGoogleCloud},
	}

	_, err := uniqueTUIProvider(det)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "multiple providers are active (aws, googlecloud)")
	assert.Contains(t, msg, "suve aws --tui")
	assert.Contains(t, msg, "suve gcloud --tui")
	// Azure is not active, so it must not be offered as a candidate command.
	assert.NotContains(t, msg, "suve azure --tui")
}

// TestUniqueTUIProvider_NoneActive pins the zero-active error: it lists every
// explicit provider form.
func TestUniqueTUIProvider_NoneActive(t *testing.T) {
	t.Parallel()

	_, err := uniqueTUIProvider(detect.Result{})
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "no provider is active")

	for _, want := range []string{"suve aws --tui", "suve gcloud --tui", "suve azure --tui"} {
		assert.Contains(t, msg, want)
	}
}

// TestActiveTUIProviders_UnionStableOrder pins that the union across service
// axes is deduplicated and returned in the stable AWS, Google Cloud, Azure
// order regardless of the order the axes list them.
func TestActiveTUIProviders_UnionStableOrder(t *testing.T) {
	t.Parallel()

	det := detect.Result{
		ParamActive:  []provider.Provider{provider.ProviderAzure, provider.ProviderAWS},
		SecretActive: []provider.Provider{provider.ProviderGoogleCloud, provider.ProviderAzure},
		StageActive:  []provider.Provider{provider.ProviderAWS},
	}

	got := activeTUIProviders(det)

	assert.Equal(t, []provider.Provider{
		provider.ProviderAWS,
		provider.ProviderGoogleCloud,
		provider.ProviderAzure,
	}, got)
}

// TestHydrateTUIScope_FillsFromEnv pins that empty resource fields hydrate from
// the environment (flag values would already be set on the scope and win).
func TestHydrateTUIScope_FillsFromEnv(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "proj-from-env")

	got := hydrateTUIScope(provider.Scope{Provider: provider.ProviderGoogleCloud})
	assert.Equal(t, "proj-from-env", got.ProjectID)
}

// TestValidateTUIScope pins the per-provider scope requirements that produce a
// launch error before the alt-screen takes over.
func TestValidateTUIScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scope   provider.Scope
		wantErr string
	}{
		{name: "aws needs nothing", scope: provider.Scope{Provider: provider.ProviderAWS}},
		{name: "gcloud with project ok", scope: provider.GoogleCloudScope("p")},
		{name: "gcloud without project", scope: provider.Scope{Provider: provider.ProviderGoogleCloud}, wantErr: "Google Cloud project"},
		{name: "azure vault ok", scope: provider.AzureKeyVaultScope("v")},
		{name: "azure store ok", scope: provider.AzureAppConfigScope("s")},
		{name: "azure neither", scope: provider.Scope{Provider: provider.ProviderAzure}, wantErr: "Azure Key Vault or App Configuration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateTUIScope(tt.scope)
			if tt.wantErr == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
