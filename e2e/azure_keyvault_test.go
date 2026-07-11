//go:build e2e

//nolint:paralleltest // E2E subtests share state and run sequentially, not in parallel
package e2e_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	commands "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/azure"
	"github.com/mpyw/suve/internal/provider"
	azureprovider "github.com/mpyw/suve/internal/provider/azure"
	"github.com/mpyw/suve/internal/provider/detect"
)

// setupAzureKeyVault skips the test unless the Key Vault emulator endpoint is
// configured, and pins a dummy vault name. The endpoint is provided by the CI
// job / make target and read by the provider adapter's emulator seam; the vault
// name is required by the CLI but ignored by the emulator (a single default
// vault is served at the endpoint).
func setupAzureKeyVault(t *testing.T) {
	t.Helper()

	if os.Getenv(azureprovider.KeyVaultEndpointEnvVar) == "" {
		t.Skipf("%s not set; skipping Azure Key Vault e2e", azureprovider.KeyVaultEndpointEnvVar)
	}

	t.Setenv("AZURE_KEYVAULT_NAME", "suve-e2e")
}

// runAzureSecret runs `suve azure secret <args...>` in-process through the azure
// command group (whose Before hooks resolve the vault from AZURE_KEYVAULT_NAME)
// and returns stdout.
func runAzureSecret(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{azure.Command()},
	}

	full := append([]string{"suve", "azure", "secret"}, args...)
	err := app.Run(t.Context(), full)

	return outBuf.String(), err
}

// setAzureKeyVaultStagingKey pins a deterministic staging encryption key for the
// working store that `stage edit`/`stage apply` use, so those commands resolve a
// key from the env instead of the OS keychain (which would block the test on an
// interactive macOS prompt). Any valid base64-standard 32-byte value works; it
// only has to be stable within the test.
func setAzureKeyVaultStagingKey(t *testing.T) {
	t.Helper()
	// EnvStagingKey ("SUVE_STAGING_KEY") is defined in an internal keyprovider
	// package the e2e module cannot import; use the literal name.
	t.Setenv("SUVE_STAGING_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
}

// runAzureStageCapture is like runAzureStage but also returns stderr, where the
// apply runner prints per-entry conflict warnings.
func runAzureStageCapture(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{azure.Command()},
	}

	full := append([]string{"suve", "azure", "stage"}, args...)
	err = app.Run(t.Context(), full)

	return outBuf.String(), errBuf.String(), err
}

// TestAzureKeyVaultStage_ConflictDetected exercises live conflict detection on
// Key Vault (whose FetchLastModified is real, unlike App Configuration's no-op).
// Staging an edit records the current version's LastModified as the conflict
// base; an out-of-band write then makes the remote newer, so apply must reject
// the write and report the conflict under the bare name (empty namespace -> no
// [namespace] badge, via EntryKey.Label()). This is the end-to-end guard for the
// resolver-based CheckConflicts path from #441 on a namespaced-capable provider.
func TestAzureKeyVaultStage_ConflictDetected(t *testing.T) {
	setupAzureKeyVault(t)
	setAzureKeyVaultStagingKey(t)
	setupTempHome(t)

	const name = "suve-e2e-kv-conflict-detected"

	cleanup := func() { _, _ = runAzureSecret(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", name, "original")
	require.NoError(t, err)

	// Stage an edit: the edit use case fetches the current version and records its
	// LastModified as the conflict base.
	_, err = runAzureStage(t, "secret", "edit", name, "staged-value")
	require.NoError(t, err)

	// Key Vault LastModified is second-granular; sleep past a second boundary so
	// the out-of-band write lands strictly after the recorded base.
	time.Sleep(1100 * time.Millisecond)

	// Modify the secret out-of-band (a new version created directly, bypassing
	// staging) so the remote is now newer than the staged base.
	_, err = runAzureSecret(t, "update", "--yes", name, "external-value")
	require.NoError(t, err)

	// Apply must detect the conflict, block the write, and report the bare name.
	_, stderr, err := runAzureStageCapture(t, "secret", "apply", "--yes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict")
	// The bare name immediately followed by ':' proves EntryKey.Label() rendered
	// no namespace badge for the empty-namespace Key Vault entry.
	assert.Contains(t, stderr, "conflict detected for "+name+":")

	// The write was blocked: the value is still the out-of-band one, not staged.
	got, err := runAzureSecret(t, "show", "--raw", name)
	require.NoError(t, err)
	assert.Equal(t, "external-value", got)
}

// TestAzureKeyVaultStage_IgnoreConflictsOverrides is the conflict scenario with
// --ignore-conflicts: the same out-of-band modification is present, but apply
// must skip the conflict check and force the staged value through.
func TestAzureKeyVaultStage_IgnoreConflictsOverrides(t *testing.T) {
	setupAzureKeyVault(t)
	setAzureKeyVaultStagingKey(t)
	setupTempHome(t)

	const name = "suve-e2e-kv-conflict-ignore"

	cleanup := func() { _, _ = runAzureSecret(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", name, "original")
	require.NoError(t, err)

	_, err = runAzureStage(t, "secret", "edit", name, "staged-value")
	require.NoError(t, err)

	time.Sleep(1100 * time.Millisecond)

	_, err = runAzureSecret(t, "update", "--yes", name, "external-value")
	require.NoError(t, err)

	// --ignore-conflicts bypasses the check: the apply succeeds despite the newer
	// remote.
	stdout, _, err := runAzureStageCapture(t, "secret", "apply", "--yes", "--ignore-conflicts")
	require.NoError(t, err)
	assert.Contains(t, stdout, name)

	got, err := runAzureSecret(t, "show", "--raw", name)
	require.NoError(t, err)
	assert.Equal(t, "staged-value", got)
}

// TestAzureKeyVaultStage_NoFalseConflict guards the other direction: staging an
// edit and applying WITHOUT any out-of-band modification must not raise a false
// conflict — the remote's LastModified still equals the recorded base.
func TestAzureKeyVaultStage_NoFalseConflict(t *testing.T) {
	setupAzureKeyVault(t)
	setAzureKeyVaultStagingKey(t)
	setupTempHome(t)

	const name = "suve-e2e-kv-no-false-conflict"

	cleanup := func() { _, _ = runAzureSecret(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", name, "original")
	require.NoError(t, err)

	_, err = runAzureStage(t, "secret", "edit", name, "staged-value")
	require.NoError(t, err)

	// No out-of-band modification between staging and apply.
	stdout, stderr, err := runAzureStageCapture(t, "secret", "apply", "--yes")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "conflict")
	assert.Contains(t, stdout, name)

	got, err := runAzureSecret(t, "show", "--raw", name)
	require.NoError(t, err)
	assert.Equal(t, "staged-value", got)
}

// TestAzureKeyVault_FullWorkflow exercises the azure secret commands against a
// local Key Vault emulator (no real Azure account). It is skipped unless
// SUVE_AZURE_KEYVAULT_ENDPOINT points at a running emulator — see the
// azure-keyvault service in compose.yaml and the `e2e-azure-keyvault` make
// target.
//
// Key Vault secrets are versioned by opaque id; this test relies on Git-like
// ~SHIFT to reach older versions without depending on the concrete version id.
func TestAzureKeyVault_FullWorkflow(t *testing.T) {
	setupAzureKeyVault(t)

	const name = "suve-e2e-kv-secret"

	// Best-effort cleanup from a previous run.
	_, _ = runAzureSecret(t, "delete", "--yes", name)

	t.Run("create", func(t *testing.T) {
		stdout, err := runAzureSecret(t, "create", name, "initial-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, name)
	})

	t.Run("show", func(t *testing.T) {
		stdout, err := runAzureSecret(t, "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "initial-value", stdout)
	})

	t.Run("update-adds-version", func(t *testing.T) {
		// Key Vault version timestamps are second-granular and versions are
		// ordered only by creation time (ids are opaque). Sleep past a second
		// boundary so create and update land in distinct seconds, making
		// ~SHIFT deterministic (matters for the emulator's fast round-trips).
		time.Sleep(1100 * time.Millisecond)

		_, err := runAzureSecret(t, "update", "--yes", name, "updated-value")
		require.NoError(t, err)

		stdout, err := runAzureSecret(t, "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "updated-value", stdout)
	})

	t.Run("show-previous-via-shift", func(t *testing.T) {
		stdout, err := runAzureSecret(t, "show", "--raw", name+"~1")
		require.NoError(t, err)
		assert.Equal(t, "initial-value", stdout)
	})

	t.Run("log-shows-two-versions", func(t *testing.T) {
		stdout, err := runAzureSecret(t, "log", name)
		require.NoError(t, err)
		// Two versions after one update; log lists each version header.
		assert.GreaterOrEqual(t, strings.Count(stdout, "Version "), 2)
	})

	t.Run("diff", func(t *testing.T) {
		stdout, err := runAzureSecret(t, "diff", name+"~1", name)
		require.NoError(t, err)
		assert.Contains(t, stdout, "initial-value")
		assert.Contains(t, stdout, "updated-value")
	})

	t.Run("list", func(t *testing.T) {
		stdout, err := runAzureSecret(t, "list")
		require.NoError(t, err)
		assert.Contains(t, stdout, name)
	})

	// The flat `suve secret` alias must reach the same Azure Key Vault (and thus
	// the emulator). The detection logic that picks the alias target is env-only
	// and unit-tested separately; here we force it to Azure and confirm the flat
	// command path resolves the vault and hits the emulator.
	t.Run("flat-alias-reaches-emulator", func(t *testing.T) {
		var outBuf, errBuf bytes.Buffer

		app := commands.MakeAppWithDetect(detect.Result{Secret: provider.ProviderAzure})
		app.Writer = &outBuf
		app.ErrWriter = &errBuf

		err := app.Run(t.Context(), []string{"suve", "secret", "show", "--raw", name})
		require.NoError(t, err)
		assert.Equal(t, "updated-value", outBuf.String())
	})

	// Tag/untag against the running emulator. The adapter tags the secret's
	// CONCRETE current version (PATCH /secrets/{name}/{version}); an empty version
	// collapses to /secrets/{name}/ and is rejected 405 — the bug this guards.
	t.Run("tag", func(t *testing.T) {
		_, err := runAzureSecret(t, "tag", name, "env=prod")
		require.NoError(t, err)

		stdout, err := runAzureSecret(t, "show", name)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env")
		assert.Contains(t, stdout, "prod")

		// log shows the tag on the (current) version — tags are per version.
		logOut, err := runAzureSecret(t, "log", name)
		require.NoError(t, err)
		assert.Contains(t, logOut, "env=prod")
	})

	t.Run("untag", func(t *testing.T) {
		_, err := runAzureSecret(t, "untag", name, "env")
		require.NoError(t, err)

		stdout, err := runAzureSecret(t, "show", name)
		require.NoError(t, err)
		assert.NotContains(t, stdout, "prod")
	})

	t.Run("delete", func(t *testing.T) {
		_, err := runAzureSecret(t, "delete", "--yes", name)
		require.NoError(t, err)

		_, err = runAzureSecret(t, "show", "--raw", name)
		require.Error(t, err)
	})
}

// TestAzureKeyVault_SoftDelete exercises soft-delete recovery: delete (soft) →
// restore → the secret is readable again. Mirrors AWS Secrets Manager's
// delete/restore semantics. Force-delete/purge is intentionally unsupported for
// Key Vault, so there is nothing to exercise here.
func TestAzureKeyVault_SoftDelete(t *testing.T) {
	setupAzureKeyVault(t)
	setupTempHome(t)

	const name = "suve-e2e-kv-softdelete"

	// Best-effort cleanup: delete (soft) then restore-and-delete is unnecessary; a
	// plain delete is enough to reset for the next run.
	cleanup := func() { _, _ = runAzureSecret(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", name, "v1")
	require.NoError(t, err)

	t.Run("soft-delete-then-restore", func(t *testing.T) {
		_, err := runAzureSecret(t, "delete", "--yes", name)
		require.NoError(t, err)

		// Soft-deleted: not readable until restored.
		_, err = runAzureSecret(t, "show", "--raw", name)
		require.Error(t, err)

		_, err = runAzureSecret(t, "restore", name)
		require.NoError(t, err)

		stdout, err := runAzureSecret(t, "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "v1", stdout)
	})
}
