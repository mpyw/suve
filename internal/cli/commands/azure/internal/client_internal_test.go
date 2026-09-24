package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
)

func TestKeyVaultStagingScopeResolver(t *testing.T) {
	t.Parallel()

	_, err := KeyVaultStagingScopeResolver(t.Context())
	require.ErrorIs(t, err, staging.ErrServiceNotConfigured)

	// A store name on the context does not leak into the Key Vault bucket.
	ctx := WithStoreName(WithVaultName(t.Context(), "my-vault"), "my-store")
	got, err := KeyVaultStagingScopeResolver(ctx)
	require.NoError(t, err)
	assert.Equal(t, staging.ResolvedScope{Scope: provider.AzureKeyVaultScope("my-vault"), Target: "vault my-vault"}, got)
}

func TestAppConfigStagingScopeResolver(t *testing.T) {
	t.Parallel()

	_, err := AppConfigStagingScopeResolver(t.Context())
	require.ErrorIs(t, err, staging.ErrServiceNotConfigured)

	ctx := WithAppConfigNamespace(WithStoreName(WithVaultName(t.Context(), "my-vault"), "my-store"), "dev")
	got, err := AppConfigStagingScopeResolver(ctx)
	require.NoError(t, err)

	want := provider.AzureAppConfigScope("my-store")
	want.AppConfigNamespace = "dev"
	assert.Equal(t, staging.ResolvedScope{Scope: want, Target: "store my-store (namespace dev)"}, got)
	assert.Equal(t, "dev", AppConfigNamespace(ctx))
}
