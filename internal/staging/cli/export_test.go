package cli_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// =============================================================================
// Export / Import test harness
// =============================================================================

// setupExportImportEnv isolates HOME (so the working store lives under a temp
// dir) and pins a deterministic working-store data key (SUVE_STAGING_KEY),
// avoiding the OS keychain. It returns a fixed AWS scope.
func setupExportImportEnv(t *testing.T) provider.Scope {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUVE_STAGING_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))

	return provider.AWSScope("123456789012", "us-east-1")
}

// fixedResolver returns a ScopeResolver that always resolves to scope.
func fixedResolver(scope provider.Scope) staging.ScopeResolver {
	return func(context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Scope: scope, Target: scope.Key()}, nil
	}
}

// paramExportImportConfig builds a service-specific param CommandConfig bound to
// the given resolver.
func paramExportImportConfig(resolver staging.ScopeResolver) stgcli.CommandConfig {
	return stgcli.CommandConfig{
		CommandName:   "param",
		ItemName:      "parameter",
		ParserFactory: staging.AWSParamParserFactory,
		ScopeResolver: resolver,
	}
}

// secretExportImportConfig builds a service-specific secret CommandConfig bound
// to the given resolver.
func secretExportImportConfig(resolver staging.ScopeResolver) stgcli.CommandConfig {
	return stgcli.CommandConfig{
		CommandName:   "secret",
		ItemName:      "secret",
		ParserFactory: staging.AWSSecretParserFactory,
		ScopeResolver: resolver,
	}
}

// runLeafCmd runs a leaf command (export/import) through a minimal app harness,
// returning stdout, stderr and the run error. stdin may be nil.
func runLeafCmd(t *testing.T, cmd *cli.Command, stdin io.Reader, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Reader:    stdin,
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{cmd},
	}

	fullArgs := append([]string{"suve", cmd.Name}, args...)
	err = app.Run(t.Context(), fullArgs)

	return outBuf.String(), errBuf.String(), err
}

// stageEntry stages a single entry in the working store for scope.
func stageEntry(t *testing.T, scope provider.Scope, svc staging.Service, name, value string) {
	t.Helper()

	store, err := file.NewWorkingStore(scope)
	require.NoError(t, err)
	require.NoError(t, store.StageEntry(t.Context(), svc, staging.EntryKey{Name: name}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(value),
		StagedAt:  time.Now(),
	}))
}

// workingState reads the current working state for scope (keep=true).
func workingState(t *testing.T, scope provider.Scope) *staging.State {
	t.Helper()

	store, err := file.NewWorkingStore(scope)
	require.NoError(t, err)

	state, err := store.Drain(t.Context(), "", true)
	require.NoError(t, err)

	return state
}

// =============================================================================
// Export tests
// =============================================================================

//nolint:paralleltest // uses t.Setenv (HOME/SUVE_STAGING_KEY); cannot run in parallel
func TestGlobalExport(t *testing.T) {
	t.Run("global export writes only non-empty services and clears working", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")

		stdout, stderr, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), nil, dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "exported")
		// Plaintext (non-TTY, no passphrase) → warning.
		assert.Contains(t, stderr, "plain text")

		// param.json written, secret.json skipped (no staged secrets).
		_, err = os.Stat(filepath.Join(dir, "param.json"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(dir, "secret.json"))
		assert.True(t, os.IsNotExist(err))

		// Working area cleared.
		assert.True(t, workingState(t, scope).IsEmpty())
	})

	t.Run("global export both services", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")
		stageEntry(t, scope, staging.ServiceSecret, "my-secret", "sval")

		dir := filepath.Join(t.TempDir(), "backup")

		_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), nil, dir)
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(dir, "param.json"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(dir, "secret.json"))
		require.NoError(t, err)
	})

	t.Run("--keep preserves the working area", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")

		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), nil, dir, "--keep")
		require.NoError(t, err)
		assert.Contains(t, stdout, "kept in the working staging area")

		assert.False(t, workingState(t, scope).IsEmpty())
	})

	t.Run("encrypted via --passphrase-stdin", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")

		stdin := bytes.NewBufferString("pw123\n")
		stdout, stderr, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), stdin, dir, "--passphrase-stdin")
		require.NoError(t, err)
		assert.Contains(t, stdout, "encrypted")
		assert.NotContains(t, stderr, "plain text")

		env, err := file.ReadEnvelopeFile(filepath.Join(dir, "param.json"))
		require.NoError(t, err)
		enc, err := env.IsEncryptedPayload()
		require.NoError(t, err)
		assert.True(t, enc)
	})

	t.Run("nothing to export", func(t *testing.T) {
		scope := setupExportImportEnv(t)

		dir := filepath.Join(t.TempDir(), "backup")

		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), nil, dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "No staged changes to export.")

		_, err = os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("missing dir argument", func(t *testing.T) {
		scope := setupExportImportEnv(t)

		_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage")
	})

	t.Run("overwrite refused in non-TTY without --yes", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "param.json"), []byte("{}"), 0o600))

		_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), nil, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exist")
	})

	t.Run("overwrite allowed with --yes", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "param.json"), []byte("{}"), 0o600))

		_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), nil, dir, "--yes")
		require.NoError(t, err)

		env, err := file.ReadEnvelopeFile(filepath.Join(dir, "param.json"))
		require.NoError(t, err)
		assert.Equal(t, "param", env.Service)
	})

	// Regression for #471: with --passphrase-stdin against a pre-existing target,
	// the overwrite confirmation must NOT read the passphrase line as a y/N answer
	// and silently cancel at exit 0. Without --yes it errors clearly; the passphrase
	// line is never consumed as a confirmation.
	t.Run("--passphrase-stdin against existing target errors instead of silently cancelling", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "param.json"), []byte("{}"), 0o600))

		stdin := bytes.NewBufferString("pw123\n")
		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), stdin, dir, "--passphrase-stdin")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exist")
		assert.Contains(t, err.Error(), "--yes")
		// It must NOT report a successful/cancelled no-op on stdout.
		assert.NotContains(t, stdout, "Operation cancelled.")
	})

	// With --passphrase-stdin AND --yes, the overwrite is allowed and the passphrase
	// line is consumed by the passphrase reader (not the confirmation), producing an
	// encrypted envelope.
	t.Run("--passphrase-stdin with --yes overwrites and encrypts using the stdin passphrase", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "param.json"), []byte("{}"), 0o600))

		stdin := bytes.NewBufferString("pw123\n")
		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), stdin, dir, "--passphrase-stdin", "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, "encrypted")

		env, err := file.ReadEnvelopeFile(filepath.Join(dir, "param.json"))
		require.NoError(t, err)
		enc, err := env.IsEncryptedPayload()
		require.NoError(t, err)
		assert.True(t, enc)
	})
}

//nolint:paralleltest // uses t.Setenv (HOME/SUVE_STAGING_KEY); cannot run in parallel
func TestServiceExport(t *testing.T) {
	t.Run("service-specific export to a file", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")
		stageEntry(t, scope, staging.ServiceSecret, "my-secret", "sval")

		fpath := filepath.Join(t.TempDir(), "out", "param.json")

		cmd := stgcli.NewExportCommand(paramExportImportConfig(fixedResolver(scope)))
		_, _, err := runLeafCmd(t, cmd, nil, fpath)
		require.NoError(t, err)

		env, err := file.ReadEnvelopeFile(fpath)
		require.NoError(t, err)
		assert.Equal(t, "param", env.Service)

		// Only the param service was cleared; the secret remains.
		state := workingState(t, scope)
		assert.True(t, state.ExtractService(staging.ServiceParam).IsEmpty())
		assert.False(t, state.ExtractService(staging.ServiceSecret).IsEmpty())
	})

	t.Run("nothing to export for this service", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceSecret, "my-secret", "sval")

		fpath := filepath.Join(t.TempDir(), "param.json")

		cmd := stgcli.NewExportCommand(paramExportImportConfig(fixedResolver(scope)))
		stdout, _, err := runLeafCmd(t, cmd, nil, fpath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "No staged changes to export.")

		_, err = os.Stat(fpath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("missing file argument", func(t *testing.T) {
		scope := setupExportImportEnv(t)

		cmd := stgcli.NewExportCommand(paramExportImportConfig(fixedResolver(scope)))
		_, _, err := runLeafCmd(t, cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage")
	})
}
