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

	commands "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/azure"
	"github.com/mpyw/suve/internal/provider"
	azureprovider "github.com/mpyw/suve/internal/provider/azure"
	"github.com/mpyw/suve/internal/provider/detect"
)

// setupAzureAppConfig skips the test unless the App Configuration emulator
// connection string is configured, and pins a dummy store name. The connection
// string is provided by the CI job / make target and read by the provider
// adapter's emulator seam; the store name is required by the CLI but ignored by
// the emulator (the endpoint is embedded in the connection string).
func setupAzureAppConfig(t *testing.T) {
	t.Helper()

	if os.Getenv(azureprovider.AppConfigConnStringEnvVar) == "" {
		t.Skipf("%s not set; skipping Azure App Configuration e2e", azureprovider.AppConfigConnStringEnvVar)
	}

	t.Setenv("AZURE_APPCONFIG_NAME", "suve-e2e")
}

// runAzureParam runs `suve azure param <args...>` in-process through the azure
// command group (whose Before hooks resolve the store from AZURE_APPCONFIG_NAME)
// and returns stdout.
func runAzureParam(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{azure.Command()},
	}

	full := append([]string{"suve", "azure", "param"}, args...)
	err := app.Run(t.Context(), full)

	return outBuf.String(), err
}

// TestAzureAppConfig_FullWorkflow exercises the azure param commands against a
// local App Configuration emulator (no real Azure account). It is skipped unless
// SUVE_AZURE_APPCONFIG_CONNECTION_STRING points at a running emulator — see the
// azure-appconfig service in compose.yaml and the `e2e-azure` make target.
//
// App Configuration is UNVERSIONED: there is no version history, so this test
// exercises create/show/update/list/tag/delete but no version-specific paths.
func TestAzureAppConfig_FullWorkflow(t *testing.T) {
	setupAzureAppConfig(t)

	const name = "suve/e2e/azure/param"

	// Best-effort cleanup from a previous run.
	_, _ = runAzureParam(t, "delete", "--yes", name)

	t.Run("create", func(t *testing.T) {
		stdout, err := runAzureParam(t, "create", name, "initial-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, name)
	})

	t.Run("show", func(t *testing.T) {
		stdout, err := runAzureParam(t, "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "initial-value", stdout)
	})

	t.Run("update-replaces-value", func(t *testing.T) {
		_, err := runAzureParam(t, "update", "--yes", name, "updated-value")
		require.NoError(t, err)

		stdout, err := runAzureParam(t, "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "updated-value", stdout)
	})

	t.Run("list", func(t *testing.T) {
		stdout, err := runAzureParam(t, "list")
		require.NoError(t, err)
		assert.Contains(t, stdout, name)
	})

	// The flat `suve param` alias must reach the same Azure App Configuration
	// store (and thus the emulator). The detection logic that picks the alias
	// target is env-only and unit-tested separately; here we force it to Azure
	// and confirm the flat command path resolves the store and hits the emulator.
	t.Run("flat-alias-reaches-emulator", func(t *testing.T) {
		var outBuf, errBuf bytes.Buffer

		app := commands.MakeAppWithDetect(detect.Result{Param: provider.ProviderAzure})
		app.Writer = &outBuf
		app.ErrWriter = &errBuf

		err := app.Run(t.Context(), []string{"suve", "param", "show", "--raw", name})
		require.NoError(t, err)
		assert.Equal(t, "updated-value", outBuf.String())
	})

	// Note: tag/untag are intentionally not exercised here. Azure App
	// Configuration tags are not writable via the azappconfig SDK, so the
	// adapter rejects tag mutation with a clear error by design.

	t.Run("delete", func(t *testing.T) {
		_, err := runAzureParam(t, "delete", "--yes", name)
		require.NoError(t, err)

		_, err = runAzureParam(t, "show", "--raw", name)
		require.Error(t, err)
	})
}

// TestAzureAppConfig_Namespace exercises the --namespace / --ns axis (#381
// Phase 1) end-to-end against the emulator. It doubles as the empirical check
// that the emulator honors App Configuration labels: (key, label) pairs are
// stored and filtered independently, so the same key under two namespaces holds
// two distinct values and a namespace-scoped list hides the others.
func TestAzureAppConfig_Namespace(t *testing.T) {
	setupAzureAppConfig(t)

	// A key that exists under BOTH the null (default) namespace and "dev", to
	// prove label isolation on read; plus a key that exists ONLY under "dev", to
	// prove list isolation.
	const (
		shared  = "suve/e2e/azure/ns/shared"
		devOnly = "suve/e2e/azure/ns/dev-only"
	)

	// Best-effort cleanup from a previous run (each (key, namespace) is distinct).
	_, _ = runAzureParam(t, "delete", "--yes", shared)
	_, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", shared)
	_, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", devOnly)

	t.Cleanup(func() {
		_, _ = runAzureParam(t, "delete", "--yes", shared)
		_, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", shared)
		_, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", devOnly)
	})

	t.Run("create-under-namespaces", func(t *testing.T) {
		_, err := runAzureParam(t, "create", shared, "null-value")
		require.NoError(t, err)

		_, err = runAzureParam(t, "create", "--namespace", "dev", shared, "dev-value")
		require.NoError(t, err)

		_, err = runAzureParam(t, "create", "--ns", "dev", devOnly, "dev-only-value")
		require.NoError(t, err)
	})

	// The same key resolves to a different value per namespace — proof the
	// emulator keys on (key, label), not key alone.
	t.Run("show-is-namespace-scoped", func(t *testing.T) {
		stdout, err := runAzureParam(t, "show", "--raw", shared)
		require.NoError(t, err)
		assert.Equal(t, "null-value", stdout)

		stdout, err = runAzureParam(t, "show", "--raw", "--namespace", "dev", shared)
		require.NoError(t, err)
		assert.Equal(t, "dev-value", stdout)
	})

	// list is scoped: the null list hides dev-only keys; the dev list shows
	// them; --namespace "*" spans all namespaces.
	t.Run("list-is-namespace-scoped", func(t *testing.T) {
		nullList, err := runAzureParam(t, "list")
		require.NoError(t, err)
		assert.Contains(t, nullList, shared)
		assert.NotContains(t, nullList, devOnly)

		devList, err := runAzureParam(t, "list", "--namespace", "dev")
		require.NoError(t, err)
		assert.Contains(t, devList, devOnly)

		allList, err := runAzureParam(t, "list", "--namespace", "*")
		require.NoError(t, err)
		assert.Contains(t, allList, devOnly)
	})

	// A filter value (all/multiple) is rejected on a single-item op.
	t.Run("wildcard-rejected-on-single-item", func(t *testing.T) {
		_, err := runAzureParam(t, "show", "--raw", "--namespace", "*", shared)
		require.Error(t, err)

		_, err = runAzureParam(t, "show", "--raw", "--namespace", "dev,prod", shared)
		require.Error(t, err)
	})
}
