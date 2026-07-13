package capability_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
)

// findService returns the ServiceCapability for (provider, service) from the
// capability matrix, failing the test when absent.
func findService(t *testing.T, caps []capability.ProviderCapability, prov, service string) capability.ServiceCapability {
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

	return capability.ServiceCapability{}
}

// TestAll_StagingAndDeleteFlags pins the staging/delete capability values that
// drive control visibility, so a stray edit to the matrix is caught. Staging is
// available for every provider service; force-delete/recovery-window are AWS
// Secrets Manager only.
func TestAll_StagingAndDeleteFlags(t *testing.T) {
	t.Parallel()

	caps := capability.All()

	tests := []struct {
		provider          string
		service           string
		hasStaging        bool
		hasForceDelete    bool
		hasRecoveryWindow bool
		hasRestore        bool
	}{
		// Staging is available for every provider service. Restore belongs to the
		// soft-delete providers — AWS Secrets Manager AND Azure Key Vault — but
		// force-delete and the per-delete recovery window are AWS SM only: Key Vault
		// retention is a vault property and force-delete/purge is unsupported there.
		{string(provider.ProviderAWS), "param", true, false, false, false},
		{string(provider.ProviderAWS), "secret", true, true, true, true},
		{string(provider.ProviderGoogleCloud), "secret", true, false, false, false},
		{string(provider.ProviderAzure), "param", true, false, false, false},
		{string(provider.ProviderAzure), "secret", true, false, false, true},
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

// TestAll_EveryServiceHasStaging pins the invariant that the staging workflow
// applies to every provider service today.
func TestAll_EveryServiceHasStaging(t *testing.T) {
	t.Parallel()

	for _, p := range capability.All() {
		for _, s := range p.Services {
			assert.True(t, s.HasStaging, "%s/%s must have staging", p.Provider, s.Service)
		}
	}
}

// TestAll_ForceDeleteAndRecoveryWindowAWSSecretOnly pins the AWS-only invariant:
// both force-delete and the per-delete recovery window are Secrets Manager
// features. Azure Key Vault soft-deletes (Restore recovers) but neither purges
// on force nor exposes a per-delete recovery window (retention is a vault
// property), so no other provider/service may report these.
func TestAll_ForceDeleteAndRecoveryWindowAWSSecretOnly(t *testing.T) {
	t.Parallel()

	for _, p := range capability.All() {
		for _, s := range p.Services {
			isAWSSecret := p.Provider == string(provider.ProviderAWS) && s.Service == "secret"
			if !isAWSSecret {
				assert.False(t, s.HasForceDelete, "%s/%s must not offer force-delete", p.Provider, s.Service)
				assert.False(t, s.HasRecoveryWindow, "%s/%s must not have a recovery window", p.Provider, s.Service)
			}
		}
	}
}

// TestAll_AzureAppConfigUnversionedWithNamespaces pins Azure App Configuration's
// two distinguishing traits: it is unversioned (no history or specifiers) and it
// partitions keys by a namespace (the label axis).
func TestAll_AzureAppConfigUnversionedWithNamespaces(t *testing.T) {
	t.Parallel()

	svc := findService(t, capability.All(), string(provider.ProviderAzure), "param")

	assert.False(t, svc.HasVersionHistory, "App Config is unversioned")
	assert.False(t, svc.HasVersionSpecifiers, "App Config has no version specifiers")
	assert.True(t, svc.HasNamespaces, "App Config partitions keys by namespace")
	assert.True(t, svc.HasTags, "App Config tags are writable via GET-merge-PUT")
}

// TestAll_TagsPerVersionAzureKeyVaultOnly pins that per-version tags are unique
// to Azure Key Vault; every other service keeps tags at the resource level.
func TestAll_TagsPerVersionAzureKeyVaultOnly(t *testing.T) {
	t.Parallel()

	for _, p := range capability.All() {
		for _, s := range p.Services {
			isAzureKeyVault := p.Provider == string(provider.ProviderAzure) && s.Service == "secret"
			assert.Equal(t, isAzureKeyVault, s.TagsPerVersion, "%s/%s TagsPerVersion", p.Provider, s.Service)
		}
	}
}

// TestAll_RestoreSoftDeleteProvidersOnly pins that Restore is offered exactly by
// the soft-delete services: AWS Secrets Manager and Azure Key Vault.
func TestAll_RestoreSoftDeleteProvidersOnly(t *testing.T) {
	t.Parallel()

	for _, p := range capability.All() {
		for _, s := range p.Services {
			isAWSSecret := p.Provider == string(provider.ProviderAWS) && s.Service == "secret"
			isAzureKeyVault := p.Provider == string(provider.ProviderAzure) && s.Service == "secret"
			assert.Equal(t, isAWSSecret || isAzureKeyVault, s.HasRestore, "%s/%s HasRestore", p.Provider, s.Service)
		}
	}
}

// TestAll_HasNamespacesAzureAppConfigOnly pins that the namespace axis is unique
// to Azure App Configuration among all services.
func TestAll_HasNamespacesAzureAppConfigOnly(t *testing.T) {
	t.Parallel()

	for _, p := range capability.All() {
		for _, s := range p.Services {
			isAzureAppConfig := p.Provider == string(provider.ProviderAzure) && s.Service == "param"
			assert.Equal(t, isAzureAppConfig, s.HasNamespaces, "%s/%s HasNamespaces", p.Provider, s.Service)
		}
	}
}

// TestAll_ProviderShape pins the provider axis: which providers exist, in order,
// with the services and scope fields they offer.
func TestAll_ProviderShape(t *testing.T) {
	t.Parallel()

	caps := capability.All()

	type shape struct {
		provider    string
		scopeFields []string
		services    []string
	}

	got := make([]shape, len(caps))
	for i, p := range caps {
		svcs := make([]string, len(p.Services))
		for j, s := range p.Services {
			svcs[j] = s.Service
		}

		got[i] = shape{provider: p.Provider, scopeFields: p.ScopeFields, services: svcs}
	}

	want := []shape{
		{provider: string(provider.ProviderAWS), scopeFields: []string{}, services: []string{"param", "secret"}},
		{provider: string(provider.ProviderGoogleCloud), scopeFields: []string{"project"}, services: []string{"secret"}},
		{provider: string(provider.ProviderAzure), scopeFields: []string{}, services: []string{"param", "secret"}},
	}

	assert.Equal(t, want, got)
}
