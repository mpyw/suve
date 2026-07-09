//go:build e2e

//nolint:paralleltest // E2E subtests share state and run sequentially, not in parallel
package e2e_test

import (
	"bytes"
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
