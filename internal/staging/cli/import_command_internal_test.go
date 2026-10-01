// White-box tests of import_command.go.
//declscope:namespace importCommand

package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
)

// globalImportCmd builds the global import command for a fixed scope resolver.
// The global command re-anchors via the config's per-service factories; these
// tests exercise same-scope and refused cross-scope paths, so a bare resolver
// (no factories) is enough.
func importCommandGlobalImportCmd(resolver staging.ScopeResolver) *cli.Command {
	return NewGlobalImportCommand(GlobalConfig{ScopeResolver: resolver})
}

// exportDir stages the given entries, exports them to a fresh directory (which
// clears the working area), and returns the directory. It is the setup shared by
// most import tests.
func importCommandExportDir(t *testing.T, scope provider.Scope, stage func()) string {
	t.Helper()

	stage()

	dir := filepath.Join(t.TempDir(), "backup")
	_, _, err := exportRunLeafCmd(t, globalExportCmd(scope), nil, dir)
	require.NoError(t, err)

	return dir
}

//nolint:paralleltest // uses t.Setenv (HOME/SUVE_STAGING_KEY); cannot run in parallel
func TestGlobalImport(t *testing.T) {
	t.Run("round-trip restores the working area", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := importCommandExportDir(t, scope, func() {
			exportStageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")
			exportStageEntry(t, scope, staging.ServiceSecret, "my-secret", "sval")
		})

		require.True(t, exportWorkingState(t, scope).IsEmpty())

		stdout, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil, dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "imported")

		state := exportWorkingState(t, scope)
		assert.False(t, state.ExtractService(staging.ServiceParam).IsEmpty())
		assert.False(t, state.ExtractService(staging.ServiceSecret).IsEmpty())
	})

	t.Run("merge combines imported with existing", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := importCommandExportDir(t, scope, func() {
			exportStageEntry(t, scope, staging.ServiceParam, "/app/param1", "v1")
		})

		// New change in the working area.
		exportStageEntry(t, scope, staging.ServiceParam, "/app/param2", "v2")

		stdout, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil, dir, "--merge")
		require.NoError(t, err)
		assert.Contains(t, stdout, "merged")

		entries := exportWorkingState(t, scope).Entries[staging.ServiceParam]
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param1"})
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param2"})
	})

	t.Run("overwrite replaces the working area for present services", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := importCommandExportDir(t, scope, func() {
			exportStageEntry(t, scope, staging.ServiceParam, "/app/param1", "v1")
		})

		exportStageEntry(t, scope, staging.ServiceParam, "/app/param2", "v2")

		_, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil, dir, "--overwrite")
		require.NoError(t, err)

		entries := exportWorkingState(t, scope).Entries[staging.ServiceParam]
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param1"})
		assert.NotContains(t, entries, staging.EntryKey{Name: "/app/param2"})
	})

	t.Run("partial dir (only param.json) imports param and skips secret", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := importCommandExportDir(t, scope, func() {
			exportStageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")
		})

		// Only param.json exists in the dir.
		stdout, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil, dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "imported")

		state := exportWorkingState(t, scope)
		assert.False(t, state.ExtractService(staging.ServiceParam).IsEmpty())
		assert.True(t, state.ExtractService(staging.ServiceSecret).IsEmpty())
	})

	t.Run("global import rejects a mislabeled file (filename vs header)", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		exportStageEntry(t, scope, staging.ServiceSecret, "my-secret", "sval")

		// Write a secret envelope to <dir>/param.json (wrong filename for the header).
		dir := t.TempDir()
		fpath := filepath.Join(dir, "param.json")
		_, _, err := exportRunLeafCmd(t, NewExportCommand(secretExportImportConfig(exportFixedResolver(scope))), nil, fpath)
		require.NoError(t, err)

		_, _, err = exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected")
	})

	t.Run("nothing to import (empty dir)", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := t.TempDir()

		stdout, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil, dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "No staged changes to import.")
	})

	t.Run("missing dir argument", func(t *testing.T) {
		scope := setupExportImportEnv(t)

		_, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage")
	})

	t.Run("scope mismatch refused, allowed with --allow-scope-mismatch", func(t *testing.T) {
		scopeA := setupExportImportEnv(t)
		dir := importCommandExportDir(t, scopeA, func() {
			exportStageEntry(t, scopeA, staging.ServiceParam, "/app/config", "pval")
		})

		scopeB := provider.AWSScope("999999999999", "eu-west-1")

		_, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scopeB)), nil, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), scopeA.Key())
		assert.Contains(t, err.Error(), scopeB.Key())

		// --allow-scope-mismatch overrides.
		_, _, err = exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scopeB)), nil, dir, "--allow-scope-mismatch")
		require.NoError(t, err)
		assert.False(t, exportWorkingState(t, scopeB).ExtractService(staging.ServiceParam).IsEmpty())
	})

	t.Run("legacy --force is no longer accepted on import", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := importCommandExportDir(t, scope, func() {
			exportStageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")
		})

		_, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil, dir, "--force")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "force")
	})

	t.Run("encrypted import in non-TTY without --passphrase-stdin is refused", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		exportStageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")
		_, _, err := exportRunLeafCmd(t, globalExportCmd(scope), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)

		// No --passphrase-stdin and a non-TTY writer: cannot prompt.
		_, _, err = exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(scope)), nil, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-TTY")
	})

	t.Run("encrypted import with wrong passphrase fails to decrypt", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		exportStageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")
		_, _, err := exportRunLeafCmd(t, globalExportCmd(scope), bytes.NewBufferString("right-pw\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)

		_, _, err = exportRunLeafCmd(
			t, importCommandGlobalImportCmd(exportFixedResolver(scope)), bytes.NewBufferString("wrong-pw\n"), dir, "--passphrase-stdin",
		)
		require.Error(t, err)
	})

	// Regression for #472: an encrypted import via --passphrase-stdin with a dirty
	// working area must merge (the default) without an EOF failure. The passphrase
	// read and the mode resolution must not double-buffer the single stdin stream.
	t.Run("encrypted --passphrase-stdin with dirty working area merges without EOF", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		exportStageEntry(t, scope, staging.ServiceParam, "/app/param1", "v1")

		dir := filepath.Join(t.TempDir(), "backup")
		_, _, err := exportRunLeafCmd(t, globalExportCmd(scope), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)
		require.True(t, exportWorkingState(t, scope).IsEmpty())

		// Dirty the working area so the reconcile path is exercised.
		exportStageEntry(t, scope, staging.ServiceParam, "/app/param2", "v2")

		// Only the passphrase is on stdin (no merge/overwrite answer line).
		stdout, _, err := exportRunLeafCmd(
			t, importCommandGlobalImportCmd(exportFixedResolver(scope)), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin",
		)
		require.NoError(t, err)
		assert.Contains(t, stdout, "merged")

		entries := exportWorkingState(t, scope).Entries[staging.ServiceParam]
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param1"})
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param2"})
	})

	t.Run("encrypted round-trip via --passphrase-stdin", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		exportStageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")
		_, _, err := exportRunLeafCmd(t, globalExportCmd(scope), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)
		require.True(t, exportWorkingState(t, scope).IsEmpty())

		_, _, err = exportRunLeafCmd(
			t, importCommandGlobalImportCmd(exportFixedResolver(scope)), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin",
		)
		require.NoError(t, err)
		assert.False(t, exportWorkingState(t, scope).ExtractService(staging.ServiceParam).IsEmpty())
	})
}

// TestGlobalImport_ProviderMismatch covers the #486 guard: a provider change is
// qualitatively different from an account/region change, so it must be refused
// even under the --allow-scope-mismatch scope override. Kept as its own function
// so TestGlobalImport stays under the funlen limit.
//
//nolint:paralleltest // uses t.Setenv (HOME/SUVE_STAGING_KEY); cannot run in parallel
func TestGlobalImport_ProviderMismatch(t *testing.T) {
	// Mirrors the failure scenario: an Azure App Config param envelope
	// {Provider: azure} imported into an AWS Parameter Store working area.
	setupExportImportEnv(t)

	azureScope := provider.AzureAppConfigScope("mystore")
	dir := importCommandExportDir(t, azureScope, func() {
		exportStageEntry(t, azureScope, staging.ServiceParam, "/app/config", "pval")
	})

	awsScope := provider.AWSScope("123456789012", "us-east-1")

	// --allow-scope-mismatch bypasses the scope check but the provider guard
	// still refuses.
	_, _, err := exportRunLeafCmd(t, importCommandGlobalImportCmd(exportFixedResolver(awsScope)), nil, dir, "--allow-scope-mismatch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
	assert.Contains(t, err.Error(), string(provider.ProviderAzure))
	assert.Contains(t, err.Error(), string(provider.ProviderAWS))

	// Nothing leaked into the AWS working area.
	assert.True(t, exportWorkingState(t, awsScope).IsEmpty())
}

//nolint:paralleltest // uses t.Setenv (HOME/SUVE_STAGING_KEY); cannot run in parallel
func TestServiceImport(t *testing.T) {
	t.Run("service mismatch is a hard error", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		exportStageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		fpath := filepath.Join(t.TempDir(), "param.json")
		_, _, err := exportRunLeafCmd(t, NewExportCommand(paramExportImportConfig(exportFixedResolver(scope))), nil, fpath)
		require.NoError(t, err)

		// Importing a param file through the secret command must hard-error.
		cmd := NewImportCommand(secretExportImportConfig(exportFixedResolver(scope)))
		_, _, err = exportRunLeafCmd(t, cmd, nil, fpath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "param")
		assert.Contains(t, err.Error(), "secret")
	})

	t.Run("service-specific round-trip", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		exportStageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		fpath := filepath.Join(t.TempDir(), "param.json")
		_, _, err := exportRunLeafCmd(t, NewExportCommand(paramExportImportConfig(exportFixedResolver(scope))), nil, fpath)
		require.NoError(t, err)
		require.True(t, exportWorkingState(t, scope).ExtractService(staging.ServiceParam).IsEmpty())

		cmd := NewImportCommand(paramExportImportConfig(exportFixedResolver(scope)))
		_, _, err = exportRunLeafCmd(t, cmd, nil, fpath)
		require.NoError(t, err)
		assert.False(t, exportWorkingState(t, scope).ExtractService(staging.ServiceParam).IsEmpty())
	})

	t.Run("missing file is an error", func(t *testing.T) {
		scope := setupExportImportEnv(t)

		cmd := NewImportCommand(paramExportImportConfig(exportFixedResolver(scope)))
		_, _, err := exportRunLeafCmd(t, cmd, nil, filepath.Join(t.TempDir(), "nope.json"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// =============================================================================
// importModeChooser tests
// =============================================================================

// =============================================================================
// State.TotalCount (preserved from the removed stash-drop tests)
// =============================================================================

func TestState_TotalCount(t *testing.T) {
	t.Parallel()

	t.Run("nil state", func(t *testing.T) {
		t.Parallel()

		var s *staging.State
		assert.Equal(t, 0, s.TotalCount())
	})

	t.Run("entries and tags", func(t *testing.T) {
		t.Parallel()

		s := staging.NewEmptyState()
		s.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{}
		s.Tags[staging.ServiceParam][staging.EntryKey{Name: "/app/config2"}] = staging.TagEntry{}
		s.Entries[staging.ServiceSecret][staging.EntryKey{Name: "secret"}] = staging.Entry{}
		assert.Equal(t, 3, s.TotalCount())
	})
}
