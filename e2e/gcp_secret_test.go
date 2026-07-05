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
	"github.com/mpyw/suve/internal/cli/commands/gcloud"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
	"github.com/mpyw/suve/internal/provider/gcp"
)

// setupGCP skips the test unless the emulator endpoint is configured, and pins a
// project id. The endpoint env var itself is provided by the CI job / make
// target and read by the provider adapter's emulator seam.
func setupGCP(t *testing.T) {
	t.Helper()

	if os.Getenv(gcp.EmulatorEnvVar) == "" {
		t.Skipf("%s not set; skipping GCP Secret Manager e2e", gcp.EmulatorEnvVar)
	}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "suve-e2e")
}

// runGcloud runs `suve gcloud <args...>` in-process through the gcloud command
// group (whose Before hook resolves the project from GOOGLE_CLOUD_PROJECT) and
// returns stdout.
func runGcloud(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{gcloud.Command()},
	}

	full := append([]string{"suve", "gcloud"}, args...)
	err := app.Run(t.Context(), full)

	return outBuf.String(), err
}

// TestGCPSecret_FullWorkflow exercises the gcloud secret commands against a
// local Secret Manager emulator (no real Google Cloud account). It is skipped
// unless SUVE_GCP_SECRETMANAGER_ENDPOINT points at a running emulator — see the
// gcp-secretmanager service in compose.yaml and the `e2e-gcp` make target.
func TestGCPSecret_FullWorkflow(t *testing.T) {
	setupGCP(t)

	const name = "suve-e2e-gcp-secret"

	// Best-effort cleanup from a previous run.
	_, _ = runGcloud(t, "secret", "delete", "--yes", name)

	t.Run("create", func(t *testing.T) {
		stdout, err := runGcloud(t, "secret", "create", name, "initial-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, name)
	})

	t.Run("show", func(t *testing.T) {
		stdout, err := runGcloud(t, "secret", "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "initial-value", stdout)
	})

	t.Run("update-adds-version", func(t *testing.T) {
		_, err := runGcloud(t, "secret", "update", "--yes", name, "updated-value")
		require.NoError(t, err)

		stdout, err := runGcloud(t, "secret", "show", "--raw", name)
		require.NoError(t, err)
		assert.Equal(t, "updated-value", stdout)
	})

	t.Run("log-shows-two-versions", func(t *testing.T) {
		stdout, err := runGcloud(t, "secret", "log", name)
		require.NoError(t, err)
		// Google Cloud versions are integers; after one update there are two.
		assert.Contains(t, stdout, "1")
		assert.Contains(t, stdout, "2")
	})

	t.Run("show-specific-version", func(t *testing.T) {
		stdout, err := runGcloud(t, "secret", "show", "--raw", name+"#1")
		require.NoError(t, err)
		assert.Equal(t, "initial-value", stdout)
	})

	t.Run("diff", func(t *testing.T) {
		stdout, err := runGcloud(t, "secret", "diff", name+"#1", name+"#2")
		require.NoError(t, err)
		assert.Contains(t, stdout, "initial-value")
		assert.Contains(t, stdout, "updated-value")
	})

	t.Run("list", func(t *testing.T) {
		stdout, err := runGcloud(t, "secret", "list")
		require.NoError(t, err)
		assert.Contains(t, stdout, name)
	})

	// The flat `suve secret` alias must reach the same Google Cloud store (and
	// thus the emulator). The detection logic that picks the alias target is
	// env-only and unit-tested separately; here we force it to Google Cloud and
	// confirm the flat command path resolves the project and hits the emulator.
	t.Run("flat-alias-reaches-emulator", func(t *testing.T) {
		var outBuf, errBuf bytes.Buffer

		app := commands.MakeAppWithDetect(detect.Result{Secret: provider.ProviderGoogleCloud})
		app.Writer = &outBuf
		app.ErrWriter = &errBuf

		err := app.Run(t.Context(), []string{"suve", "secret", "show", "--raw", name})
		require.NoError(t, err)
		assert.Equal(t, "updated-value", outBuf.String())
	})

	t.Run("tag-and-untag-labels", func(t *testing.T) {
		_, err := runGcloud(t, "secret", "tag", name, "env=test")
		require.NoError(t, err)

		stdout, err := runGcloud(t, "secret", "show", name)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env")

		_, err = runGcloud(t, "secret", "untag", name, "env")
		require.NoError(t, err)
	})

	t.Run("delete", func(t *testing.T) {
		_, err := runGcloud(t, "secret", "delete", "--yes", name)
		require.NoError(t, err)

		_, err = runGcloud(t, "secret", "show", "--raw", name)
		require.Error(t, err)
	})
}
