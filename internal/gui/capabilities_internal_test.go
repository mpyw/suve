//go:build production || dev

package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
)

// findService returns the ServiceCapability for (provider, service) from the
// capability descriptor, failing the test when absent.
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

// TestApp_Capabilities_StagingAndDeleteFlags pins the staging/delete capability
// values that drive control visibility, so a stray edit to providers.go is
// caught. Staging is AWS-only until #270; force-delete/recovery-window are AWS
// Secrets Manager only.
func TestApp_Capabilities_StagingAndDeleteFlags(t *testing.T) {
	t.Parallel()

	caps := (&App{}).Capabilities()

	tests := []struct {
		provider          string
		service           string
		hasStaging        bool
		hasForceDelete    bool
		hasRecoveryWindow bool
		hasRestore        bool
	}{
		{string(provider.ProviderAWS), "param", true, false, false, false},
		{string(provider.ProviderAWS), "secret", true, true, true, true},
		{string(provider.ProviderGoogleCloud), "secret", false, false, false, false},
		{string(provider.ProviderAzure), "param", false, false, false, false},
		{string(provider.ProviderAzure), "secret", false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.service, func(t *testing.T) {
			t.Parallel()

			svc := findService(t, caps, tt.provider, tt.service)
			assert.Equal(t, tt.hasStaging, svc.HasStaging, "HasStaging")
			assert.Equal(t, tt.hasForceDelete, svc.HasForceDelete, "HasForceDelete")
			assert.Equal(t, tt.hasRecoveryWindow, svc.HasRecoveryWindow, "HasRecoveryWindow")
			assert.Equal(t, tt.hasRestore, svc.HasRestore, "HasRestore")
		})
	}
}

// TestApp_Capabilities_OnlyAWSHasStaging is a stronger invariant: no non-AWS
// service may advertise staging until multi-provider staging lands.
func TestApp_Capabilities_OnlyAWSHasStaging(t *testing.T) {
	t.Parallel()

	for _, p := range (&App{}).Capabilities() {
		for _, s := range p.Services {
			if p.Provider != string(provider.ProviderAWS) {
				assert.False(t, s.HasStaging, "%s/%s must not have staging", p.Provider, s.Service)
			}
		}
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
