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
// caught. Staging is available for every provider service;
// force-delete/recovery-window are AWS Secrets Manager only.
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
		// Staging is now available for every provider service; force-delete
		// and recovery-window stay AWS Secrets Manager only.
		{string(provider.ProviderAWS), "param", true, false, false, false},
		{string(provider.ProviderAWS), "secret", true, true, true, true},
		{string(provider.ProviderGoogleCloud), "secret", true, false, false, false},
		{string(provider.ProviderAzure), "param", true, false, false, false},
		{string(provider.ProviderAzure), "secret", true, false, false, false},
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

// TestApp_Capabilities_DeleteOptionsAWSOnly pins the remaining AWS-only
// invariant: force-delete and recovery-window are Secrets Manager features, so
// no other provider/service may advertise them even though they now stage.
func TestApp_Capabilities_DeleteOptionsAWSOnly(t *testing.T) {
	t.Parallel()

	for _, p := range (&App{}).Capabilities() {
		for _, s := range p.Services {
			isAWSSecret := p.Provider == string(provider.ProviderAWS) && s.Service == "secret"
			if !isAWSSecret {
				assert.False(t, s.HasForceDelete, "%s/%s must not have force-delete", p.Provider, s.Service)
				assert.False(t, s.HasRecoveryWindow, "%s/%s must not have a recovery window", p.Provider, s.Service)
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
