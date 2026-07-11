//go:build e2e

//nolint:paralleltest // E2E subtests share state and run sequentially, not in parallel
package e2e_test

import (
	"bytes"
	"os"
	"path/filepath"
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
	store, err := file.NewWorkingStore(provider.AzureKeyVaultScope("suve-e2e"))
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

	store, err := file.NewWorkingStore(provider.AzureAppConfigScope("suve-e2e"))
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

// TestAzureStageGlobal_KeyVaultOnly exercises the provider-wide `suve azure stage`
// global commands (status/diff/apply) when ONLY Key Vault is connected: App
// Configuration is not configured (no AZURE_APPCONFIG_NAME), so the global
// commands must SKIP it rather than error — an unconfigured service can hold no
// staged state (#435).
func TestAzureStageGlobal_KeyVaultOnly(t *testing.T) {
	setupAzureKeyVault(t)
	setupTempHome(t)

	// Guarantee App Configuration is unconfigured regardless of ambient env.
	t.Setenv("AZURE_APPCONFIG_NAME", "")

	const name = "suve-e2e-az-global-kv-only"

	cleanup := func() { _, _ = runAzureSecret(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	store, err := file.NewWorkingStore(provider.AzureKeyVaultScope("suve-e2e"))
	require.NoError(t, err)
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: name}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("kv-only-value"), StagedAt: time.Now(),
	}))

	t.Run("status", func(t *testing.T) {
		stdout, err := runAzureStage(t, "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Key Vault")
		assert.Contains(t, stdout, name)
		// App Configuration is skipped, not reported.
		assert.NotContains(t, stdout, "App Configuration")
	})

	t.Run("diff", func(t *testing.T) {
		stdout, err := runAzureStage(t, "diff")
		require.NoError(t, err)
		assert.Contains(t, stdout, "+kv-only-value")
	})

	t.Run("apply", func(t *testing.T) {
		stdout, err := runAzureStage(t, "apply", "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, name)

		got, err := runAzureSecret(t, "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "kv-only-value", got)
	})
}

// TestAzureStageGlobal_AppConfigOnly is the mirror of the Key Vault-only case:
// only App Configuration is connected, so the global commands must SKIP Key
// Vault (no AZURE_KEYVAULT_NAME) rather than error.
func TestAzureStageGlobal_AppConfigOnly(t *testing.T) {
	setupAzureAppConfig(t)
	setupTempHome(t)

	// Guarantee Key Vault is unconfigured regardless of ambient env.
	t.Setenv("AZURE_KEYVAULT_NAME", "")

	const name = "suve/e2e/az/global/ac-only"

	cleanup := func() { _, _ = runAzureParam(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	store, err := file.NewWorkingStore(provider.AzureAppConfigScope("suve-e2e"))
	require.NoError(t, err)
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: name}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("ac-only-value"), StagedAt: time.Now(),
	}))

	t.Run("status", func(t *testing.T) {
		stdout, err := runAzureStage(t, "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "App Configuration")
		assert.Contains(t, stdout, name)
		// Key Vault is skipped, not reported.
		assert.NotContains(t, stdout, "Key Vault")
	})

	t.Run("diff", func(t *testing.T) {
		stdout, err := runAzureStage(t, "diff")
		require.NoError(t, err)
		assert.Contains(t, stdout, "+ac-only-value")
	})

	t.Run("apply", func(t *testing.T) {
		stdout, err := runAzureStage(t, "apply", "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, name)

		got, err := runAzureParam(t, "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "ac-only-value", got)
	})
}

// TestAzureStageGlobal_BothConnected exercises the global commands when BOTH Key
// Vault and App Configuration are connected: a single `suve azure stage status`/
// `apply` spans both services. It is skipped unless both emulator endpoints are
// set (the single-service e2e make targets run one emulator at a time).
func TestAzureStageGlobal_BothConnected(t *testing.T) {
	setupAzureKeyVault(t)
	setupAzureAppConfig(t)
	setupTempHome(t)

	const (
		kvName = "suve-e2e-az-global-both-secret"
		acName = "suve/e2e/az/global/both-param"
	)

	cleanup := func() {
		_, _ = runAzureSecret(t, "delete", "--yes", kvName)
		_, _ = runAzureParam(t, "delete", "--yes", acName)
	}
	cleanup()
	t.Cleanup(cleanup)

	kvStore, err := file.NewWorkingStore(provider.AzureKeyVaultScope("suve-e2e"))
	require.NoError(t, err)
	require.NoError(t, kvStore.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: kvName}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("both-secret-value"), StagedAt: time.Now(),
	}))

	acStore, err := file.NewWorkingStore(provider.AzureAppConfigScope("suve-e2e"))
	require.NoError(t, err)
	require.NoError(t, acStore.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: acName}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("both-param-value"), StagedAt: time.Now(),
	}))

	t.Run("status-spans-both", func(t *testing.T) {
		stdout, err := runAzureStage(t, "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Key Vault")
		assert.Contains(t, stdout, kvName)
		assert.Contains(t, stdout, "App Configuration")
		assert.Contains(t, stdout, acName)
	})

	t.Run("apply-spans-both", func(t *testing.T) {
		stdout, err := runAzureStage(t, "apply", "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, kvName)
		assert.Contains(t, stdout, acName)

		gotSecret, err := runAzureSecret(t, "show", "--raw", kvName)
		require.NoError(t, err)
		assert.Equal(t, "both-secret-value", gotSecret)

		gotParam, err := runAzureParam(t, "show", "--raw", acName)
		require.NoError(t, err)
		assert.Equal(t, "both-param-value", gotParam)
	})
}

// TestAzureAppConfigStage_NamespacedTags proves staged TAGS are keyed per
// (name, namespace): the SAME key tagged under the null namespace and under
// "dev" holds independent staged tag changes, `status` attributes each to its
// namespace, and `apply` writes each tag onto its own namespaced setting. This
// is the end-to-end guard for the tag-namespace-collision fix (tags used to be
// keyed by bare name). Gated on the emulator honoring tag writes.
func TestAzureAppConfigStage_NamespacedTags(t *testing.T) {
	setupAzureAppConfig(t)
	setupTempHome(t)

	if !emulatorHonorsTagWrite(t) {
		t.Skip("App Configuration emulator does not persist setting tags; skipping tag-write assertions")
	}

	const key = "suve/e2e/az/ac/stage/ns-tag-key"

	cleanup := func() {
		_, _ = runAzureParam(t, "delete", "--yes", key)
		_, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", key)
	}
	cleanup()
	t.Cleanup(cleanup)

	// The settings must exist (under each namespace) before tagging.
	_, err := runAzureParam(t, "create", key, "null-value")
	require.NoError(t, err)
	_, err = runAzureParam(t, "create", "--namespace", "dev", key, "dev-value")
	require.NoError(t, err)

	// Stage a DIFFERENT tag on the same key under each namespace.
	t.Run("stage-tags-under-two-namespaces", func(t *testing.T) {
		_, err := runAzureStage(t, "param", "tag", key, "tier=null")
		require.NoError(t, err)

		_, err = runAzureStage(t, "param", "tag", "--namespace", "dev", key, "tier=dev")
		require.NoError(t, err)
	})

	// status shows the key twice, each tag attributed to its own namespace (the
	// dev one badged) — the collision the bare-name key used to cause.
	t.Run("status-shows-both-namespaced-tags", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, key)
		assert.Contains(t, stdout, "[dev]")
	})

	// apply writes each tag onto its own namespaced setting; neither overwrites
	// the other.
	t.Run("apply-writes-each-namespaced-tag", func(t *testing.T) {
		_, err := runAzureStage(t, "param", "apply", "--yes")
		require.NoError(t, err)

		nullShow, err := runAzureParam(t, "show", key)
		require.NoError(t, err)
		assert.Contains(t, nullShow, "tier")
		assert.Contains(t, nullShow, "null")

		devShow, err := runAzureParam(t, "show", "--namespace", "dev", key)
		require.NoError(t, err)
		assert.Contains(t, devShow, "tier")
		assert.Contains(t, devShow, "dev")
	})
}

// TestAzureKeyVaultStage_ExportImport exercises the service-specific
// `azure stage secret export <file>` / `import <file>` round-trip against the
// Key Vault emulator. It uses an isolated temp HOME so the working staging area
// starts empty.
func TestAzureKeyVaultStage_ExportImport(t *testing.T) {
	setupAzureKeyVault(t)
	setupTempHome(t)

	const name = "suve-e2e-az-kv-stage-export-import"

	exportPath := filepath.Join(t.TempDir(), "secret.json")

	// Stage a create in the working staging area.
	_, err := runAzureStage(t, "secret", "add", name, "exported-value")
	require.NoError(t, err)

	t.Run("export", func(t *testing.T) {
		stdout, err := runAzureStage(t, "secret", "export", exportPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "exported")

		_, statErr := os.Stat(exportPath)
		require.NoError(t, statErr)
	})

	t.Run("working-cleared", func(t *testing.T) {
		stdout, err := runAzureStage(t, "secret", "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, name)
	})

	t.Run("import", func(t *testing.T) {
		stdout, err := runAzureStage(t, "secret", "import", exportPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "imported")
	})

	t.Run("working-restored", func(t *testing.T) {
		stdout, err := runAzureStage(t, "secret", "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Key Vault")
		assert.Contains(t, stdout, name)
	})
}

// TestAzureAppConfigStage_ExportImport exercises the service-specific
// `azure stage param export <file>` / `import <file>` round-trip against the App
// Configuration emulator. Beyond restoring the working area, it applies the
// re-imported staged param and reads the real setting back: this is the e2e
// guard for #445 (a staged App Config param must survive export -> import in the
// PARAM bucket, not be misrouted/dropped into the secret bucket).
func TestAzureAppConfigStage_ExportImport(t *testing.T) {
	setupAzureAppConfig(t)
	setupTempHome(t)

	const name = "suve/e2e/az/ac/stage/export-import"

	exportPath := filepath.Join(t.TempDir(), "param.json")

	cleanup := func() { _, _ = runAzureParam(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	// Stage a create of an App Configuration setting.
	_, err := runAzureStage(t, "param", "add", name, "exported-value")
	require.NoError(t, err)

	t.Run("export", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "export", exportPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "exported")

		_, statErr := os.Stat(exportPath)
		require.NoError(t, statErr)
	})

	t.Run("working-cleared", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, name)
	})

	t.Run("import", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "import", exportPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "imported")
	})

	// The re-imported staged param must land back in the param (App Config)
	// bucket: status attributes it to App Configuration and shows the key.
	t.Run("working-restored", func(t *testing.T) {
		stdout, err := runAzureStage(t, "param", "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "App Configuration")
		assert.Contains(t, stdout, name)
	})

	// Applying the re-imported staged param writes the real setting, proving the
	// value and per-service bucket survived the round-trip end to end (#445).
	t.Run("apply-after-roundtrip", func(t *testing.T) {
		_, err := runAzureStage(t, "param", "apply", "--yes")
		require.NoError(t, err)

		stdout, err := runAzureParam(t, "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "exported-value", stdout)
	})
}
