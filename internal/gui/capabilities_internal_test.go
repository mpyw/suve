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

// findService returns the ServiceCapability for (provider, service) from the
// capability descriptor, failing the test when absent. (The capability matrix
// now lives in internal/capability; this small lookup helper is kept local to
// the HasDescription gating test below.)
func findService(t *testing.T, caps []ProviderCapability, prov, service string) ServiceCapability {
	t.Helper()

	for _, p := range caps {
		if p.Provider != prov {
			continue
		}

		for _, s := range p.Services {
			if s.Service == service {
				return s
			}
		}
	}

	t.Fatalf("no capability for provider %q service %q", prov, service)

	return ServiceCapability{}
}

// TestApp_Capabilities_HasDescription pins the HasDescription capability that
// gates the create/edit Description input (#767): AWS Param + Secret and Google
// Cloud Secret persist a description; Azure App Configuration and Key Vault
// ignore it, so their inputs stay hidden.
func TestApp_Capabilities_HasDescription(t *testing.T) {
	t.Parallel()

	caps := (&App{}).Capabilities()

	tests := []struct {
		provider       string
		service        string
		hasDescription bool
	}{
		{string(provider.ProviderAWS), "param", true},
		{string(provider.ProviderAWS), "secret", true},
		{string(provider.ProviderGoogleCloud), "secret", true},
		{string(provider.ProviderAzure), "param", false},
		{string(provider.ProviderAzure), "secret", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.service, func(t *testing.T) {
			t.Parallel()

			svc := findService(t, caps, tt.provider, tt.service)
			assert.Equal(t, tt.hasDescription, svc.HasDescription, "HasDescription")
		})
	}
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
