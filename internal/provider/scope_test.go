package provider_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/provider"
)

func TestScope_Key(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		scope provider.Scope
		want  string
	}{
		{
			name:  "aws",
			scope: provider.AWSScope("123456789012", "ap-northeast-1"),
			want:  "aws/123456789012/ap-northeast-1",
		},
		{
			name:  "googlecloud",
			scope: provider.GoogleCloudScope("my-project"),
			want:  "googlecloud/my-project",
		},
		{
			// Vault names are globally unique, so the name alone keys the scope.
			name:  "azure keyvault",
			scope: provider.AzureKeyVaultScope("vault1"),
			want:  "azure/keyvault/vault1",
		},
		{
			name:  "azure appconfig",
			scope: provider.AzureAppConfigScope("store1"),
			want:  "azure/appconfig/store1",
		},
		{
			// Staging is per-STORE, not per-namespace: the namespace is NOT in the
			// key (each entry carries its own namespace as part of its identity,
			// see staging.CompositeEntryKey), so a named namespace keys the same
			// store directory as the null namespace.
			name: "azure appconfig namespace is not part of the key",
			scope: provider.Scope{
				Provider:           provider.ProviderAzure,
				StoreName:          "store1",
				AppConfigNamespace: "dev",
			},
			want: "azure/appconfig/store1",
		},
		{
			name:  "unknown provider",
			scope: provider.Scope{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.scope.Key())
		})
	}
}

func TestScope_SupportsService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scope      provider.Scope
		wantParam  bool
		wantSecret bool
	}{
		{
			name:       "aws supports both",
			scope:      provider.AWSScope("123456789012", "ap-northeast-1"),
			wantParam:  true,
			wantSecret: true,
		},
		{
			name:       "googlecloud secret only",
			scope:      provider.GoogleCloudScope("my-project"),
			wantParam:  false,
			wantSecret: true,
		},
		{
			name:       "azure keyvault secret only",
			scope:      provider.AzureKeyVaultScope("vault1"),
			wantParam:  false,
			wantSecret: true,
		},
		{
			name:       "azure appconfig param only",
			scope:      provider.AzureAppConfigScope("store1"),
			wantParam:  true,
			wantSecret: false,
		},
		{
			// A scope carrying BOTH a store and a vault supports BOTH services —
			// they are independent Azure resources (regression guard: a vault must
			// not suppress param support).
			name:       "azure both vault and store",
			scope:      provider.Scope{Provider: provider.ProviderAzure, StoreName: "store1", VaultName: "vault1"},
			wantParam:  true,
			wantSecret: true,
		},
		{
			name:       "unknown supports nothing",
			scope:      provider.Scope{},
			wantParam:  false,
			wantSecret: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantParam, tt.scope.SupportsService(provider.KindParam))
			assert.Equal(t, tt.wantSecret, tt.scope.SupportsService(provider.KindSecret))
		})
	}
}

func TestScope_SupportedKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		scope provider.Scope
		want  []provider.Kind
	}{
		{
			name:  "aws both in stable order",
			scope: provider.AWSScope("123456789012", "ap-northeast-1"),
			want:  []provider.Kind{provider.KindParam, provider.KindSecret},
		},
		{
			name:  "googlecloud secret only",
			scope: provider.GoogleCloudScope("my-project"),
			want:  []provider.Kind{provider.KindSecret},
		},
		{
			name:  "azure appconfig param only",
			scope: provider.AzureAppConfigScope("store1"),
			want:  []provider.Kind{provider.KindParam},
		},
		{
			name:  "unknown none",
			scope: provider.Scope{},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.scope.SupportedKinds())
		})
	}
}

func TestScope_Constructors(t *testing.T) {
	t.Parallel()

	aws := provider.AWSScope("acct", "region")
	assert.Equal(t, provider.ProviderAWS, aws.Provider)
	assert.Equal(t, "acct", aws.AccountID)
	assert.Equal(t, "region", aws.Region)

	gcloud := provider.GoogleCloudScope("proj")
	assert.Equal(t, provider.ProviderGoogleCloud, gcloud.Provider)
	assert.Equal(t, "proj", gcloud.ProjectID)

	kv := provider.AzureKeyVaultScope("vault")
	assert.Equal(t, provider.ProviderAzure, kv.Provider)
	assert.Equal(t, "vault", kv.VaultName)

	ac := provider.AzureAppConfigScope("store")
	assert.Equal(t, provider.ProviderAzure, ac.Provider)
	assert.Equal(t, "store", ac.StoreName)
}
