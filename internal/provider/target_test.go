package provider_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/provider"
)

func TestScope_Target(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		scope       provider.Scope
		wantString  string
		wantLabels  []string
		wantPending bool
	}{
		{
			name:        "aws without identity is pending",
			scope:       provider.Scope{Provider: provider.ProviderAWS},
			wantString:  "",
			wantLabels:  []string{"profile", "account", "region"},
			wantPending: true,
		},
		{
			name:       "aws with account and region",
			scope:      provider.AWSScope("123456789012", "us-east-1"),
			wantString: "account 123456789012 · region us-east-1",
			wantLabels: []string{"profile", "account", "region"},
		},
		{
			name:       "google cloud",
			scope:      provider.GoogleCloudScope("proj"),
			wantString: "project proj",
			wantLabels: []string{"project"},
		},
		{
			name:       "azure vault only",
			scope:      provider.AzureKeyVaultScope("v"),
			wantString: "vault v",
			wantLabels: []string{"vault", "store"},
		},
		{
			name: "azure store with namespace",
			scope: provider.Scope{
				Provider: provider.ProviderAzure, VaultName: "v", StoreName: "s", AppConfigNamespace: "dev",
			},
			wantString: "vault v · store s · namespace dev",
			wantLabels: []string{"vault", "store", "namespace"},
		},
		{
			name:  "unknown provider",
			scope: provider.Scope{Provider: provider.Provider("mystery")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.scope.Target()
			assert.Equal(t, tt.wantString, got.String())
			assert.Equal(t, tt.wantPending, got.Pending)

			var labels []string
			for _, s := range got.Segments {
				labels = append(labels, s.Label)
			}

			assert.Equal(t, tt.wantLabels, labels)
		})
	}
}

func TestAWSTarget(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "profile dev · account 123456789012 · region us-east-1",
		provider.AWSTarget("dev", "123456789012", "us-east-1").String())
	assert.False(t, provider.AWSTarget("dev", "123456789012", "us-east-1").Pending)
}
