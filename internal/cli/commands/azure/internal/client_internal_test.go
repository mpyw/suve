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
	assert.Equal(t, staging.ResolvedScope{Scope: provider.AzureKeyVaultScope("my-vault"), Target: provider.AzureKeyVaultScope("my-vault").Target()}, got)
}

func TestAppConfigStagingScopeResolver(t *testing.T) {
	t.Parallel()

	_, err := AppConfigStagingScopeResolver(t.Context())
	require.ErrorIs(t, err, staging.ErrServiceNotConfigured)

	ctx := WithAppConfigNamespace(WithStoreName(WithVaultName(t.Context(), "my-vault"), "my-store"), "dev")
	got, err := AppConfigStagingScopeResolver(ctx)
	require.NoError(t, err)

	want := provider.AzureAppConfigScope("my-store")
	target := want.Target()
	want.AppConfigNamespace = "dev"
	// The target names the store alone: its staging bucket holds every
	// namespace, and apply pushes them all (#994).
	assert.Equal(t, staging.ResolvedScope{Scope: want, Target: target}, got)
	assert.Equal(t, "store my-store", got.Target.String())
	assert.Equal(t, "dev", AppConfigNamespace(ctx))
}

func TestConfirmTargets(t *testing.T) {
	t.Parallel()

	assert.Empty(t, KeyVaultConfirmTarget(t.Context()))
	assert.Empty(t, AppConfigConfirmTarget(t.Context()))

	ctx := WithAppConfigNamespace(WithStoreName(WithVaultName(t.Context(), "my-vault"), "my-store"), "dev")
	assert.Equal(t, "vault my-vault", KeyVaultConfirmTarget(ctx))
	assert.Equal(t, "store my-store · namespace dev", AppConfigConfirmTarget(ctx))
}
