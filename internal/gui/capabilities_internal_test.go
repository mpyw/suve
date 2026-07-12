//go:build production || dev

package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
)

// TestApp_Capabilities_DelegatesToCapabilityPackage pins the Wails binding
// contract: the (*App).Capabilities binding returns the neutral matrix from
// internal/capability unchanged. The matrix-content invariants themselves are
// asserted in internal/capability's own tests.
func TestApp_Capabilities_DelegatesToCapabilityPackage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, capability.All(), (&App{}).Capabilities())
}

func TestApp_ParamTypeOptions_ScopeAware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		provider  provider.Provider
		wantEmpty bool
	}{
		{name: "aws returns ssm types", provider: provider.ProviderAWS, wantEmpty: false},
		{name: "azure app config has no types", provider: provider.ProviderAzure, wantEmpty: true},
		{name: "googlecloud has no param types", provider: provider.ProviderGoogleCloud, wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appWithProvider(tt.provider)

			opts := app.ParamTypeOptions()
			if tt.wantEmpty {
				assert.Empty(t, opts)
			} else {
				require.NotEmpty(t, opts)
				assert.Contains(t, opts, "String")
				assert.Contains(t, opts, "SecureString")
				assert.Contains(t, opts, "StringList")
			}
		})
	}
}
