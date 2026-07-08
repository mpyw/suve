//go:build e2e

//nolint:paralleltest // E2E subtests share state and run sequentially, not in parallel
package e2e_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/azure"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// runAzureStage runs `suve azure stage <args...>` in-process through the azure
// command group and returns stdout.
func runAzureStage(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{azure.Command()},
	}

	full := append([]string{"suve", "azure", "stage"}, args...)
	err := app.Run(t.Context(), full)

	return outBuf.String(), err
}

// TestAzureKeyVaultStage_Workflow exercises `suve azure stage secret`
// status/diff/apply for update, create, and delete against a local Key Vault
// emulator (lowkey-vault). Skipped unless the Key Vault emulator endpoint is set.
func TestAzureKeyVaultStage_Workflow(t *testing.T) {
	setupAzureKeyVault(t)
	setupTempHome(t)

	const (
		updateName = "suve-e2e-az-kv-stage-update"
		createName = "suve-e2e-az-kv-stage-create"
		deleteName = "suve-e2e-az-kv-stage-delete"
	)

	cleanup := func() {
		_, _ = runAzureSecret(t, "delete", "--yes", updateName)
		_, _ = runAzureSecret(t, "delete", "--yes", createName)
		_, _ = runAzureSecret(t, "delete", "--yes", deleteName)
	}
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", updateName, "original")
	require.NoError(t, err)
	_, err = runAzureSecret(t, "create", deleteName, "to-be-deleted")
	require.NoError(t, err)

	// The Key Vault staging scope is keyed by the globally-unique vault name
	// alone; the emulator setup pins it to "suve-e2e".
	store, err := file.NewStore(provider.AzureKeyVaultScope("suve-e2e"))
	require.NoError(t, err)

	now := time.Now()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: updateName}, staging.Entry{
		Operation: staging.OperationUpdate, Value: lo.ToPtr("staged-value"), StagedAt: now,
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: createName}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("created-value"), StagedAt: now,
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: deleteName}, staging.Entry{
		Operation: staging.OperationDelete, StagedAt: now,
	}))

	t.Run("status", func(t *testing.T) {
		stdout, err := runAzureStage(t, "secret", "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Key Vault")
		assert.Contains(t, stdout, updateName)
		assert.Contains(t, stdout, createName)
		assert.Contains(t, stdout, deleteName)
	})

	t.Run("diff", func(t *testing.T) {
		stdout, err := runAzureStage(t, "secret", "diff")
		require.NoError(t, err)
		assert.Contains(t, stdout, "-original")
		assert.Contains(t, stdout, "+staged-value")
		assert.Contains(t, stdout, "+created-value")
	})

	t.Run("apply", func(t *testing.T) {
		stdout, err := runAzureStage(t, "secret", "apply", "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, updateName)
		assert.Contains(t, stdout, createName)
		assert.Contains(t, stdout, deleteName)
	})

	t.Run("verify", func(t *testing.T) {
		stdout, err := runAzureSecret(t, "show", "--raw", updateName)
		require.NoError(t, err)
		assert.Equal(t, "staged-value", stdout)

		stdout, err = runAzureSecret(t, "show", "--raw", createName)
		require.NoError(t, err)
		assert.Equal(t, "created-value", stdout)

		_, err = runAzureSecret(t, "show", "--raw", deleteName)
		require.Error(t, err)
	})

	t.Run("status-empty-after-apply", func(t *testing.T) {
		stdout, err := runAzureStage(t, "secret", "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, updateName)
	})
}

// TestAzureAppConfigStage_Workflow exercises `suve azure stage param`
// status/diff/apply against a local App Configuration emulator. App
// Configuration is unversioned (last-write-wins) and tags are unsupported, so
// only value operations are exercised.
func TestAzureAppConfigStage_Workflow(t *testing.T) {
	setupAzureAppConfig(t)
	setupTempHome(t)

	const (
		updateName = "suve/e2e/az/ac/stage/update"
		createName = "suve/e2e/az/ac/stage/create"
	)

	cleanup := func() {
		_, _ = runAzureParam(t, "delete", "--yes", updateName)
		_, _ = runAzureParam(t, "delete", "--yes", createName)
	}
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureParam(t, "create", updateName, "original")
	require.NoError(t, err)

	store, err := file.NewStore(provider.AzureAppConfigScope("suve-e2e"))
	require.NoError(t, err)

	now := time.Now()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: updateName}, staging.Entry{
		Operation: staging.OperationUpdate, Value: lo.ToPtr("staged-value"), StagedAt: now,
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: createName}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("created-value"), StagedAt: now,
	}))

	t.Run("status", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "App Configuration")
		assert.Contains(t, stdout, updateName)
		assert.Contains(t, stdout, createName)
	})

	t.Run("diff", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "diff")
		require.NoError(t, err)
		assert.Contains(t, stdout, "-original")
		assert.Contains(t, stdout, "+staged-value")
		assert.Contains(t, stdout, "+created-value")
	})

	t.Run("apply", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "apply", "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, updateName)
		assert.Contains(t, stdout, createName)
	})

	t.Run("verify", func(t *testing.T) {
		stdout, err := runAzureParam(t, "show", "--raw", updateName)
		require.NoError(t, err)
		assert.Equal(t, "staged-value", stdout)

		stdout, err = runAzureParam(t, "show", "--raw", createName)
		require.NoError(t, err)
		assert.Equal(t, "created-value", stdout)
	})
}

// TestAzureAppConfigStage_Namespaces exercises the per-store staging model
// (#431): a single `param.json` holds creates staged under different namespaces,
// `status` shows them all (whatever the --namespace filter), and `apply` writes
// each under its own namespace.
func TestAzureAppConfigStage_Namespaces(t *testing.T) {
	setupAzureAppConfig(t)
	setupTempHome(t)

	const key = "suve/e2e/az/ac/stage/ns-key"

	cleanup := func() {
		_, _ = runAzureParam(t, "delete", "--yes", key)
		_, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", key)
	}
	cleanup()
	t.Cleanup(cleanup)

	// Stage a create under the null namespace and the same key under "dev". Both
	// land in the one per-store staging file (namespace is part of each entry's
	// identity, not of the on-disk path).
	t.Run("stage-under-two-namespaces", func(t *testing.T) {
		_, err := runAzureStage(t, "param", "add", key, "null-value")
		require.NoError(t, err)

		_, err = runAzureStage(t, "param", "add", "--namespace", "dev", key, "dev-value")
		require.NoError(t, err)
	})

	// status (no --namespace) lists BOTH staged entries and marks the dev one with
	// its namespace — the detection that per-namespace buckets used to break.
	t.Run("status-shows-both-namespaces", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, key)
		assert.Contains(t, stdout, "[dev]")
	})

	t.Run("apply-writes-each-namespace", func(t *testing.T) {
		_, err := runAzureStage(t, "param", "apply", "--yes")
		require.NoError(t, err)

		// The null-namespace value and the dev value are independent.
		stdout, err := runAzureParam(t, "show", "--raw", key)
		require.NoError(t, err)
		assert.Equal(t, "null-value", stdout)

		stdout, err = runAzureParam(t, "show", "--raw", "--namespace", "dev", key)
		require.NoError(t, err)
		assert.Equal(t, "dev-value", stdout)
	})
}
