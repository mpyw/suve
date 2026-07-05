//go:build e2e

//nolint:paralleltest // E2E subtests share state and run sequentially, not in parallel
package e2e_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/azure"
	azureprovider "github.com/mpyw/suve/internal/provider/azure"
)

// setupAzureAppConfig skips the test unless the App Configuration connection
// string (pointing at the emulator) is set. It also pins a store name to
// satisfy the CLI's --store-name requirement; the adapter ignores it when a
// connection string is present.
func setupAzureAppConfig(t *testing.T) {
	t.Helper()

	if os.Getenv(azureprovider.AppConfigConnStringEnvVar) == "" {
		t.Skipf("%s not set; skipping Azure App Configuration e2e", azureprovider.AppConfigConnStringEnvVar)
	}

	t.Setenv("AZURE_APPCONFIG_NAME", "emulator")
}

// runAzure runs `suve azure <args...>` in-process through the azure command
// group and returns stdout.
func runAzure(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{azure.Command()},
	}

	full := append([]string{"suve", "azure"}, args...)
	err := app.Run(t.Context(), full)

	return outBuf.String(), err
}

// TestAzureAppConfig_FullWorkflow exercises the azure param (App Configuration)
// commands against a local emulator (no real Azure account). It is skipped
// unless AZURE_APPCONFIG_CONNECTION_STRING points at a running emulator — see
// the azure-appconfig service in compose.yaml and the `e2e-azure` make target.
// App Configuration is unversioned, so there are no log/diff/history checks.
func TestAzureAppConfig_FullWorkflow(t *testing.T) {
	setupAzureAppConfig(t)

	const key = "suve-e2e-appconfig-key"

	// Best-effort cleanup from a previous run.
	_, _ = runAzure(t, "param", "delete", "--yes", key)

	t.Run("create", func(t *testing.T) {
		stdout, err := runAzure(t, "param", "create", key, "initial-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, key)
	})

	t.Run("show", func(t *testing.T) {
		stdout, err := runAzure(t, "param", "show", "--raw", key)
		require.NoError(t, err)
		assert.Equal(t, "initial-value", stdout)
	})

	t.Run("update", func(t *testing.T) {
		_, err := runAzure(t, "param", "update", "--yes", key, "updated-value")
		require.NoError(t, err)

		stdout, err := runAzure(t, "param", "show", "--raw", key)
		require.NoError(t, err)
		assert.Equal(t, "updated-value", stdout)
	})

	t.Run("list", func(t *testing.T) {
		stdout, err := runAzure(t, "param", "list")
		require.NoError(t, err)
		assert.Contains(t, stdout, key)
	})

	t.Run("delete", func(t *testing.T) {
		_, err := runAzure(t, "param", "delete", "--yes", key)
		require.NoError(t, err)

		_, err = runAzure(t, "param", "show", "--raw", key)
		require.Error(t, err)
	})
}
