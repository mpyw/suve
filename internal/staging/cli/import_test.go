package cli_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	usestaging "github.com/mpyw/suve/internal/usecase/staging"
)

// exportDir stages the given entries, exports them to a fresh directory (which
// clears the working area), and returns the directory. It is the setup shared by
// most import tests.
func exportDir(t *testing.T, scope provider.Scope, stage func()) string {
	t.Helper()

	stage()

	dir := filepath.Join(t.TempDir(), "backup")
	_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), nil, dir)
	require.NoError(t, err)

	return dir
}

//nolint:paralleltest // uses t.Setenv (HOME/SUVE_STAGING_KEY); cannot run in parallel
func TestGlobalImport(t *testing.T) {
	t.Run("round-trip restores the working area", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := exportDir(t, scope, func() {
			stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")
			stageEntry(t, scope, staging.ServiceSecret, "my-secret", "sval")
		})

		require.True(t, workingState(t, scope).IsEmpty())

		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), nil, dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "imported")

		state := workingState(t, scope)
		assert.False(t, state.ExtractService(staging.ServiceParam).IsEmpty())
		assert.False(t, state.ExtractService(staging.ServiceSecret).IsEmpty())
	})

	t.Run("merge combines imported with existing", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := exportDir(t, scope, func() {
			stageEntry(t, scope, staging.ServiceParam, "/app/param1", "v1")
		})

		// New change in the working area.
		stageEntry(t, scope, staging.ServiceParam, "/app/param2", "v2")

		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), nil, dir, "--merge")
		require.NoError(t, err)
		assert.Contains(t, stdout, "merged")

		entries := workingState(t, scope).Entries[staging.ServiceParam]
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param1"})
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param2"})
	})

	t.Run("overwrite replaces the working area for present services", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := exportDir(t, scope, func() {
			stageEntry(t, scope, staging.ServiceParam, "/app/param1", "v1")
		})

		stageEntry(t, scope, staging.ServiceParam, "/app/param2", "v2")

		_, _, err := runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), nil, dir, "--overwrite")
		require.NoError(t, err)

		entries := workingState(t, scope).Entries[staging.ServiceParam]
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param1"})
		assert.NotContains(t, entries, staging.EntryKey{Name: "/app/param2"})
	})

	t.Run("partial dir (only param.json) imports param and skips secret", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := exportDir(t, scope, func() {
			stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")
		})

		// Only param.json exists in the dir.
		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), nil, dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "imported")

		state := workingState(t, scope)
		assert.False(t, state.ExtractService(staging.ServiceParam).IsEmpty())
		assert.True(t, state.ExtractService(staging.ServiceSecret).IsEmpty())
	})

	t.Run("global import rejects a mislabeled file (filename vs header)", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceSecret, "my-secret", "sval")

		// Write a secret envelope to <dir>/param.json (wrong filename for the header).
		dir := t.TempDir()
		fpath := filepath.Join(dir, "param.json")
		_, _, err := runLeafCmd(t, stgcli.NewExportCommand(secretExportImportConfig(fixedResolver(scope))), nil, fpath)
		require.NoError(t, err)

		_, _, err = runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), nil, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected")
	})

	t.Run("nothing to import (empty dir)", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		dir := t.TempDir()

		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), nil, dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "No staged changes to import.")
	})

	t.Run("missing dir argument", func(t *testing.T) {
		scope := setupExportImportEnv(t)

		_, _, err := runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage")
	})

	t.Run("scope mismatch refused, allowed with --force", func(t *testing.T) {
		scopeA := setupExportImportEnv(t)
		dir := exportDir(t, scopeA, func() {
			stageEntry(t, scopeA, staging.ServiceParam, "/app/config", "pval")
		})

		scopeB := provider.AWSScope("999999999999", "eu-west-1")

		_, _, err := runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scopeB)), nil, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), scopeA.Key())
		assert.Contains(t, err.Error(), scopeB.Key())

		// --force overrides.
		_, _, err = runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scopeB)), nil, dir, "--force")
		require.NoError(t, err)
		assert.False(t, workingState(t, scopeB).ExtractService(staging.ServiceParam).IsEmpty())
	})

	t.Run("encrypted import in non-TTY without --passphrase-stdin is refused", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")
		_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)

		// No --passphrase-stdin and a non-TTY writer: cannot prompt.
		_, _, err = runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), nil, dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-TTY")
	})

	t.Run("encrypted import with wrong passphrase fails to decrypt", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")
		_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), bytes.NewBufferString("right-pw\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)

		_, _, err = runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), bytes.NewBufferString("wrong-pw\n"), dir, "--passphrase-stdin")
		require.Error(t, err)
	})

	// Regression for #472: an encrypted import via --passphrase-stdin with a dirty
	// working area must merge (the default) without an EOF failure. The passphrase
	// read and the mode resolution must not double-buffer the single stdin stream.
	t.Run("encrypted --passphrase-stdin with dirty working area merges without EOF", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/param1", "v1")

		dir := filepath.Join(t.TempDir(), "backup")
		_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)
		require.True(t, workingState(t, scope).IsEmpty())

		// Dirty the working area so the reconcile path is exercised.
		stageEntry(t, scope, staging.ServiceParam, "/app/param2", "v2")

		// Only the passphrase is on stdin (no merge/overwrite answer line).
		stdout, _, err := runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)
		assert.Contains(t, stdout, "merged")

		entries := workingState(t, scope).Entries[staging.ServiceParam]
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param1"})
		assert.Contains(t, entries, staging.EntryKey{Name: "/app/param2"})
	})

	t.Run("encrypted round-trip via --passphrase-stdin", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		dir := filepath.Join(t.TempDir(), "backup")
		_, _, err := runLeafCmd(t, stgcli.NewGlobalExportCommand(fixedResolver(scope)), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)
		require.True(t, workingState(t, scope).IsEmpty())

		_, _, err = runLeafCmd(t, stgcli.NewGlobalImportCommand(fixedResolver(scope)), bytes.NewBufferString("pw123\n"), dir, "--passphrase-stdin")
		require.NoError(t, err)
		assert.False(t, workingState(t, scope).ExtractService(staging.ServiceParam).IsEmpty())
	})
}

//nolint:paralleltest // uses t.Setenv (HOME/SUVE_STAGING_KEY); cannot run in parallel
func TestServiceImport(t *testing.T) {
	t.Run("service mismatch is a hard error", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		fpath := filepath.Join(t.TempDir(), "param.json")
		_, _, err := runLeafCmd(t, stgcli.NewExportCommand(paramExportImportConfig(fixedResolver(scope))), nil, fpath)
		require.NoError(t, err)

		// Importing a param file through the secret command must hard-error.
		cmd := stgcli.NewImportCommand(secretExportImportConfig(fixedResolver(scope)))
		_, _, err = runLeafCmd(t, cmd, nil, fpath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "param")
		assert.Contains(t, err.Error(), "secret")
	})

	t.Run("service-specific round-trip", func(t *testing.T) {
		scope := setupExportImportEnv(t)
		stageEntry(t, scope, staging.ServiceParam, "/app/config", "pval")

		fpath := filepath.Join(t.TempDir(), "param.json")
		_, _, err := runLeafCmd(t, stgcli.NewExportCommand(paramExportImportConfig(fixedResolver(scope))), nil, fpath)
		require.NoError(t, err)
		require.True(t, workingState(t, scope).ExtractService(staging.ServiceParam).IsEmpty())

		cmd := stgcli.NewImportCommand(paramExportImportConfig(fixedResolver(scope)))
		_, _, err = runLeafCmd(t, cmd, nil, fpath)
		require.NoError(t, err)
		assert.False(t, workingState(t, scope).ExtractService(staging.ServiceParam).IsEmpty())
	})

	t.Run("missing file is an error", func(t *testing.T) {
		scope := setupExportImportEnv(t)

		cmd := stgcli.NewImportCommand(paramExportImportConfig(fixedResolver(scope)))
		_, _, err := runLeafCmd(t, cmd, nil, filepath.Join(t.TempDir(), "nope.json"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// =============================================================================
// ImportModeChooser tests
// =============================================================================

func TestImportModeChooser_ChooseMode(t *testing.T) {
	t.Parallel()

	t.Run("overwrite flag takes precedence", func(t *testing.T) {
		t.Parallel()

		chooser := &stgcli.ImportModeChooser{Stderr: &bytes.Buffer{}, Stdout: &bytes.Buffer{}}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{OverwriteFlag: true, MergeFlag: true, HasChanges: true, IsTTY: true})
		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, usestaging.ImportModeOverwrite, result.Mode)
	})

	t.Run("merge flag takes precedence over prompt", func(t *testing.T) {
		t.Parallel()

		chooser := &stgcli.ImportModeChooser{Stderr: &bytes.Buffer{}, Stdout: &bytes.Buffer{}}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{MergeFlag: true, HasChanges: true, IsTTY: true})
		require.NoError(t, err)
		assert.Equal(t, usestaging.ImportModeMerge, result.Mode)
	})

	t.Run("defaults to merge with no changes", func(t *testing.T) {
		t.Parallel()

		chooser := &stgcli.ImportModeChooser{Stderr: &bytes.Buffer{}, Stdout: &bytes.Buffer{}}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{HasChanges: false, IsTTY: true})
		require.NoError(t, err)
		assert.Equal(t, usestaging.ImportModeMerge, result.Mode)
	})

	t.Run("defaults to merge in non-TTY", func(t *testing.T) {
		t.Parallel()

		chooser := &stgcli.ImportModeChooser{Stderr: &bytes.Buffer{}, Stdout: &bytes.Buffer{}}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{HasChanges: true, ItemCount: 5, IsTTY: false})
		require.NoError(t, err)
		assert.Equal(t, usestaging.ImportModeMerge, result.Mode)
	})

	t.Run("--yes skips the prompt and defaults to merge", func(t *testing.T) {
		t.Parallel()

		chooser := &stgcli.ImportModeChooser{Stderr: &bytes.Buffer{}, Stdout: &bytes.Buffer{}}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{SkipPrompt: true, HasChanges: true, ItemCount: 2, IsTTY: true})
		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, usestaging.ImportModeMerge, result.Mode)
	})

	// Regression for #472: with --passphrase-stdin, stdin carries the passphrase,
	// not an interactive answer. The mode must resolve to the default (Merge)
	// without prompting, so the passphrase read and a would-be confirmation prompt
	// never contend over the same stdin.
	t.Run("--passphrase-stdin skips the prompt and defaults to merge", func(t *testing.T) {
		t.Parallel()

		stderr := &bytes.Buffer{}
		// A Prompter whose stdin would error if read, proving no prompt occurs.
		chooser := &stgcli.ImportModeChooser{
			Prompter: &confirm.Prompter{Stdin: bytes.NewBufferString(""), Stdout: &bytes.Buffer{}, Stderr: stderr},
			Stderr:   stderr,
			Stdout:   &bytes.Buffer{},
		}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{PassphraseStdin: true, HasChanges: true, ItemCount: 2, IsTTY: true})
		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, usestaging.ImportModeMerge, result.Mode)
		assert.NotContains(t, stderr.String(), "How do you want to proceed?")
	})

	t.Run("prompt selects merge", func(t *testing.T) {
		t.Parallel()

		stderr := &bytes.Buffer{}
		chooser := &stgcli.ImportModeChooser{
			Prompter: &confirm.Prompter{Stdin: bytes.NewBufferString("1\n"), Stdout: &bytes.Buffer{}, Stderr: stderr},
			Stderr:   stderr,
			Stdout:   &bytes.Buffer{},
		}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{HasChanges: true, ItemCount: 3, IsTTY: true})
		require.NoError(t, err)
		assert.Equal(t, usestaging.ImportModeMerge, result.Mode)
		assert.Contains(t, stderr.String(), "3 staged change(s)")
	})

	t.Run("prompt selects overwrite", func(t *testing.T) {
		t.Parallel()

		chooser := &stgcli.ImportModeChooser{
			Prompter: &confirm.Prompter{Stdin: bytes.NewBufferString("2\n"), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}},
			Stderr:   &bytes.Buffer{},
			Stdout:   &bytes.Buffer{},
		}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{HasChanges: true, ItemCount: 3, IsTTY: true})
		require.NoError(t, err)
		assert.Equal(t, usestaging.ImportModeOverwrite, result.Mode)
	})

	t.Run("prompt selects cancel", func(t *testing.T) {
		t.Parallel()

		chooser := &stgcli.ImportModeChooser{
			Prompter: &confirm.Prompter{Stdin: bytes.NewBufferString("3\n"), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}},
			Stderr:   &bytes.Buffer{},
			Stdout:   &bytes.Buffer{},
		}
		result, err := chooser.ChooseMode(stgcli.ImportModeInput{HasChanges: true, ItemCount: 3, IsTTY: true})
		require.NoError(t, err)
		assert.True(t, result.Cancelled)
	})
}

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
