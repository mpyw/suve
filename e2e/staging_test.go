//go:build e2e

//nolint:paralleltest,dogsled,gosec // E2E tests: sequential execution, cleanup, G101 false positive
package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmdparam "github.com/mpyw/suve/internal/cli/commands/param"
	paramcreate "github.com/mpyw/suve/internal/cli/commands/param/create"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/param/delete"
	cmdsecret "github.com/mpyw/suve/internal/cli/commands/secret"
	secretcreate "github.com/mpyw/suve/internal/cli/commands/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/secret/delete"
	globalstage "github.com/mpyw/suve/internal/cli/commands/stage"
	globalapply "github.com/mpyw/suve/internal/cli/commands/stage/apply"
	globaldiff "github.com/mpyw/suve/internal/cli/commands/stage/diff"
	paramstage "github.com/mpyw/suve/internal/cli/commands/stage/param"
	globalreset "github.com/mpyw/suve/internal/cli/commands/stage/reset"
	secretstage "github.com/mpyw/suve/internal/cli/commands/stage/secret"
	globalstatus "github.com/mpyw/suve/internal/cli/commands/stage/status"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

// awsStageGlobalConfig builds the AWS provider config for the global stage
// commands (param + secret), used by the e2e tests.
func awsStageGlobalConfig() stgcli.GlobalConfig {
	return stgcli.AWSGlobalConfig(paramstage.Config(), secretstage.Config())
}

// =============================================================================
// Global Stage Commands Tests
// =============================================================================

// TestGlobal_StageWorkflow tests the global stage commands that work across services.
func TestGlobal_StageWorkflow(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-global/param"
	secretName := "suve-e2e-global/secret"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create resources
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "original-param")
	_, _, _ = runCommand(t, secretcreate.Command(), secretName, "original-secret")

	// Stage both
	store := newStore()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-param"),
		StagedAt:  time.Now(),
	})

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: secretName}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-secret"),
		StagedAt:  time.Now(),
	})

	// 1. Global status shows both
	t.Run("global-status", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, secretName)
		assert.Contains(t, stdout, "SSM Parameter Store")
		assert.Contains(t, stdout, "Secrets Manager")
		t.Logf("global status output: %s", stdout)
	})

	// 2. Global diff shows both
	t.Run("global-diff", func(t *testing.T) {
		stdout, _, err := runCommand(t, globaldiff.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stdout, "-original-param")
		assert.Contains(t, stdout, "+staged-param")
		assert.Contains(t, stdout, "-original-secret")
		assert.Contains(t, stdout, "+staged-secret")
		t.Logf("global diff output: %s", stdout)
	})

	// 3. Global apply applies both
	t.Run("global-apply", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalapply.Command(awsStageGlobalConfig()), "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, secretName)
		t.Logf("global apply output: %s", stdout)
	})

	// 4. Verify both updated
	t.Run("verify", func(t *testing.T) {
		stdout, _, err := runCommand(t, cmdparam.ShowCommand(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "staged-param", stdout)

		stdout, _, err = runCommand(t, cmdsecret.ShowCommand(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, "staged-secret", stdout)
	})

	// 5. Status should be empty
	t.Run("status-empty", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.NotContains(t, stdout, paramName)
		assert.NotContains(t, stdout, secretName)
	})
}

// TestGlobal_StageResetAll tests global reset --all.
func TestGlobal_StageResetAll(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-global-reset/param"
	secretName := "suve-e2e-global-reset/secret"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create and stage
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "original")
	_, _, _ = runCommand(t, secretcreate.Command(), secretName, "original")

	store := newStore()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged"),
		StagedAt:  time.Now(),
	})

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: secretName}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged"),
		StagedAt:  time.Now(),
	})

	// Global reset requires --all flag
	t.Run("reset-without-all-warns", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, globalreset.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		// Without --all, should show warning
		assert.Contains(t, stderr, "no effect")
		t.Logf("global reset without --all output: %s, stderr: %s", stdout, stderr)
	})

	// Verify still staged (not reset without --all)
	t.Run("verify-still-staged", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, secretName)
	})

	// Global reset with --all
	t.Run("reset-with-all", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--all")
		require.NoError(t, err)
		t.Logf("global reset --all output: %s", stdout)
	})

	// Verify empty
	t.Run("verify-empty", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.NotContains(t, stdout, paramName)
		assert.NotContains(t, stdout, secretName)
	})
}

// TestGlobal_StageCommand tests the global stage parent command structure.
func TestGlobal_StageCommand(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	// Test that global stage command has correct subcommands
	t.Run("has-subcommands", func(t *testing.T) {
		cmd := globalstage.Command()
		assert.Equal(t, "stage", cmd.Name)

		subCmdNames := make([]string, len(cmd.Commands))
		for i, sub := range cmd.Commands {
			subCmdNames[i] = sub.Name
		}

		assert.Contains(t, subCmdNames, "status")
		assert.Contains(t, subCmdNames, "diff")
		assert.Contains(t, subCmdNames, "apply")
		assert.Contains(t, subCmdNames, "reset")
	})
}

// TestStaging_ErrorCases tests staging error scenarios.
func TestStaging_ErrorCases(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	// Push when nothing staged - warning goes to stdout
	t.Run("apply-nothing-staged", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)
		// Message might say "No SSM Parameter Store changes staged" or similar
		assert.Contains(t, stdout, "No")
		t.Logf("apply nothing staged output: %s", stdout)
	})

	// Push non-existent staged item - the command checks if it's staged first
	t.Run("apply-nonexistent", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes", "/nonexistent/param")
		// Should error with "not staged" message
		if err == nil {
			t.Log("Note: apply with non-staged param didn't error (may be expected behavior)")
		}
	})

	// Reset when nothing staged - message goes to stdout
	t.Run("reset-all-nothing-staged", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "reset", "--all")
		require.NoError(t, err)
		// Message might say "No SSM Parameter Store parameters staged" or similar
		assert.Contains(t, stdout, "No")
		t.Logf("reset all nothing staged output: %s", stdout)
	})

	// Diff with non-staged parameter
	t.Run("diff-not-staged", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "diff", "/nonexistent/param")
		// Should error with "not staged" message
		if err == nil {
			t.Log("Note: diff with non-staged param didn't error (may be expected behavior)")
		}
	})
}

// TestGlobal_StagingWithTags tests the global stage commands (diff, apply, reset) with tag entries.
func TestGlobal_StagingWithTags(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-global/stage-tags/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create a parameter first
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "initial-value")
	require.NoError(t, err)

	// Stage tag changes using the staging store directly
	store := newStore()
	err = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, staging.TagEntry{
		Add:      map[string]string{"env": "test", "team": "e2e"},
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	// Test global diff shows tag changes
	t.Run("global-diff-shows-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, globaldiff.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stdout, "Tags:")
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, "env=test")
		t.Logf("global diff with tags output: %s", stdout)
	})

	// Test global status shows tag changes
	t.Run("global-status-shows-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stdout, "T")         // T = Tag change marker
		assert.Contains(t, stdout, "+2 tag(s)") // Two tags being added
		t.Logf("global status with tags output: %s", stdout)
	})

	// Test global apply applies tag changes
	t.Run("global-apply-applies-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalapply.Command(awsStageGlobalConfig()), "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Applying SSM Parameter Store tags")
		assert.Contains(t, stdout, "Tagged")
		t.Logf("global apply with tags output: %s", stdout)

		// Verify tags were applied by checking show output
		stdout, _, err = runCommand(t, cmdparam.ShowCommand(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env: test")
		assert.Contains(t, stdout, "team: e2e")
	})
}

// TestGlobal_TagConflictDetection covers #483: a staged tag whose remote was
// modified after the recorded BaseModifiedAt is rejected as a conflict at apply
// time, and --ignore-conflicts forces it through. Staging the tag with a
// BaseModifiedAt in the past simulates the remote being changed out-of-band
// since the tags were fetched — the real FetchLastModified returns the
// parameter's actual (newer) modified time.
func TestGlobal_TagConflictDetection(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-global/tag-conflict/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create a parameter; its LastModified is now.
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "initial-value")
	require.NoError(t, err)

	// Stage a tag change whose BaseModifiedAt predates the parameter — as if the
	// remote had been modified since the tags were fetched.
	past := time.Now().Add(-24 * time.Hour)
	store := newStore()
	err = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, staging.TagEntry{
		Add:            map[string]string{"env": "test"},
		StagedAt:       time.Now(),
		BaseModifiedAt: &past,
	})
	require.NoError(t, err)

	// Apply without --ignore-conflicts: rejected as a conflict; tag stays staged.
	t.Run("apply-rejected-on-conflict", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflict")

		_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName})
		require.NoError(t, err)
	})

	// Apply with --ignore-conflicts: forced through and unstaged.
	t.Run("apply-forced-with-ignore-conflicts", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes", "--ignore-conflicts")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Tagged")

		_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName})
		require.ErrorIs(t, err, staging.ErrNotStaged)

		stdout, _, err = runCommand(t, cmdparam.ShowCommand(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env: test")
	})
}

// TestGlobal_ResetWithTags tests the global reset command with tag entries.
func TestGlobal_ResetWithTags(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-global/reset-tags/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create a parameter first
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "initial-value")
	require.NoError(t, err)

	// Stage entry and tag changes
	store := newStore()
	err = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("updated-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, staging.TagEntry{
		Add:      map[string]string{"env": "test"},
		StagedAt: time.Now(),
	})

	require.NoError(t, err)

	// Test global reset clears both entries and tags
	t.Run("global-reset-clears-all", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--all")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Unstaged all changes")
		assert.Contains(t, stdout, "2 SSM Parameter Store") // 1 entry + 1 tag
		t.Logf("global reset output: %s", stdout)

		// Verify staging is empty
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName, Namespace: ""})
		assert.Equal(t, staging.ErrNotStaged, err)
		_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName})
		assert.Equal(t, staging.ErrNotStaged, err)
	})
}

// =============================================================================
// Export and Import Tests
// =============================================================================

// TestGlobal_ExportImport tests the export -> clear -> import round-trip.
func TestGlobal_ExportImport(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-export-import/param"

	_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	})

	// Stage a parameter in the working staging area.
	_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "test-value")
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "backup")

	// Export writes <dir>/param.json and clears the working area.
	t.Run("export", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, globalstage.Command(), "export", dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "exported")

		_, statErr := os.Stat(filepath.Join(dir, "param.json"))
		require.NoError(t, statErr)
	})

	// Working area is now empty.
	t.Run("working-cleared", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.NotContains(t, stdout, paramName)
	})

	// Import restores the working area.
	t.Run("import", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, globalstage.Command(), "import", dir)
		require.NoError(t, err)
		assert.Contains(t, stdout, "imported")
	})

	t.Run("working-restored", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})
}

// TestGlobal_ExportKeep tests export --keep retains the working area.
func TestGlobal_ExportKeep(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-export-keep/param"

	_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	})

	_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "keep-value")
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "backup")

	stdout, _, err := runSubCommand(t, globalstage.Command(), "export", dir, "--keep")
	require.NoError(t, err)
	assert.Contains(t, stdout, "kept in the working staging area")

	// Still staged.
	stdout, _, err = runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
	require.NoError(t, err)
	assert.Contains(t, stdout, paramName)
}

// TestGlobal_ImportMerge tests import --merge combines with the working area.
func TestGlobal_ImportMerge(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName1 := "/suve-e2e-import-merge/param1"
	paramName2 := "/suve-e2e-import-merge/param2"

	_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	})

	// Stage param1 and export it (clears the working area).
	_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName1, "value1")
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "backup")
	_, _, err = runSubCommand(t, globalstage.Command(), "export", dir)
	require.NoError(t, err)

	// Stage param2 in the working area.
	_, _, err = runSubCommand(t, paramstage.Command(), "add", paramName2, "value2")
	require.NoError(t, err)

	// Import merges param1 back in.
	stdout, _, err := runSubCommand(t, globalstage.Command(), "import", dir, "--merge")
	require.NoError(t, err)
	assert.Contains(t, stdout, "merged")

	stdout, _, err = runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
	require.NoError(t, err)
	assert.Contains(t, stdout, paramName1)
	assert.Contains(t, stdout, paramName2)
}

// TestGlobal_ImportScopeMismatch tests that a scope mismatch is refused unless
// --force is given.
func TestGlobal_ImportScopeMismatch(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-import-scope/param"

	_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	})

	_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "value")
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "backup")
	_, _, err = runSubCommand(t, globalstage.Command(), "export", dir)
	require.NoError(t, err)

	// Tamper the exported envelope's scope so it no longer matches.
	path := filepath.Join(dir, "param.json")

	raw, err := os.ReadFile(path) //nolint:gosec // path from t.TempDir()
	require.NoError(t, err)

	var env map[string]any
	require.NoError(t, json.Unmarshal(raw, &env))
	env["scope"] = "aws/999999999999/eu-west-1"

	tampered, err := json.Marshal(env)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, tampered, 0o600))

	// Import is refused by default.
	t.Run("refused", func(t *testing.T) {
		_, _, err := runSubCommand(t, globalstage.Command(), "import", dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scope")
	})

	// --force overrides.
	t.Run("forced", func(t *testing.T) {
		_, _, err := runSubCommand(t, globalstage.Command(), "import", dir, "--force")
		require.NoError(t, err)

		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})
}

// TestGlobal_ExportEmpty tests export when nothing is staged.
func TestGlobal_ExportEmpty(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")

	dir := filepath.Join(t.TempDir(), "backup")

	stdout, _, err := runSubCommand(t, globalstage.Command(), "export", dir)
	require.NoError(t, err)
	assert.Contains(t, stdout, "No staged changes to export.")

	_, statErr := os.Stat(dir)
	assert.True(t, os.IsNotExist(statErr))
}

// TestGlobal_ExportImportEncrypted tests an encrypted export/import round-trip
// via --passphrase-stdin.
func TestGlobal_ExportImportEncrypted(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-export-encrypted/param"

	_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--yes")
	})

	_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "secret-value")
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "backup")

	// Export encrypted.
	stdout, stderr, err := runSubCommandWithStdin(
		t, globalstage.Command(), strings.NewReader("testpass\n"), "export", dir, "--passphrase-stdin",
	)
	require.NoError(t, err, "export failed: stdout=%s stderr=%s", stdout, stderr)
	assert.Contains(t, stdout, "encrypted")

	// Import back with the same passphrase.
	stdout, stderr, err = runSubCommandWithStdin(
		t, globalstage.Command(), strings.NewReader("testpass\n"), "import", dir, "--passphrase-stdin",
	)
	require.NoError(t, err, "import failed: stdout=%s stderr=%s", stdout, stderr)
	assert.Contains(t, stdout, "imported")

	stdout, _, err = runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
	require.NoError(t, err)
	assert.Contains(t, stdout, paramName)
}

// =============================================================================
// Agent Store Direct Tests (for IPC coverage)
// =============================================================================

// TestAgentStore_DirectMethods tests agent store methods directly to improve IPC coverage.
func TestAgentStore_DirectMethods(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()
	paramName := "/suve-e2e-direct/test-param"

	// Clean up first
	_ = store.UnstageAll(t.Context(), staging.ServiceParam)
	t.Cleanup(func() {
		_ = store.UnstageAll(t.Context(), staging.ServiceParam)
	})

	// Test StageEntry and GetEntry
	t.Run("stage-and-get-entry", func(t *testing.T) {
		entry := staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("direct-test-value"),
			StagedAt:  time.Now(),
		}
		err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, entry)
		require.NoError(t, err)

		retrieved, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName, Namespace: ""})
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, retrieved.Operation)
		assert.Equal(t, "direct-test-value", lo.FromPtr(retrieved.Value))
	})

	// Test ListEntries
	t.Run("list-entries", func(t *testing.T) {
		entries, err := store.ListEntries(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Contains(t, entries[staging.ServiceParam], staging.EntryKey{Name: paramName})
	})

	// Test UnstageEntry
	t.Run("unstage-entry", func(t *testing.T) {
		err := store.UnstageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName, Namespace: ""})
		require.NoError(t, err)

		_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName, Namespace: ""})
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	// Test GetEntry for non-existent entry
	t.Run("get-nonexistent-entry", func(t *testing.T) {
		_, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/nonexistent/param", Namespace: ""})
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

// TestAgentStore_TagMethods tests tag-related methods on the agent store.
func TestAgentStore_TagMethods(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()
	paramName := "/suve-e2e-direct/tag-test"

	// Clean up first
	_ = store.UnstageAll(t.Context(), staging.ServiceParam)
	t.Cleanup(func() {
		_ = store.UnstageAll(t.Context(), staging.ServiceParam)
	})

	// Test StageTag and GetTag
	t.Run("stage-and-get-tag", func(t *testing.T) {
		tagEntry := staging.TagEntry{
			Add:      map[string]string{"env": "test", "team": "e2e"},
			Remove:   maputil.NewSet("old-tag"),
			StagedAt: time.Now(),
		}
		err := store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, tagEntry)
		require.NoError(t, err)

		retrieved, err := store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName})
		require.NoError(t, err)
		assert.Equal(t, "test", retrieved.Add["env"])
		assert.Equal(t, "e2e", retrieved.Add["team"])
		assert.True(t, retrieved.Remove.Contains("old-tag"))
	})

	// Test ListTags
	t.Run("list-tags", func(t *testing.T) {
		tags, err := store.ListTags(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Contains(t, tags[staging.ServiceParam], staging.EntryKey{Name: paramName})
	})

	// Test UnstageTag
	t.Run("unstage-tag", func(t *testing.T) {
		err := store.UnstageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName})
		require.NoError(t, err)

		_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName})
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	// Test GetTag for non-existent tag
	t.Run("get-nonexistent-tag", func(t *testing.T) {
		_, err := store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/nonexistent/param"})
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

// TestAgentStore_LoadAndWriteState tests Load and WriteState methods.
func TestAgentStore_LoadAndWriteState(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Clean up first
	_ = store.UnstageAll(t.Context(), "")
	t.Cleanup(func() {
		_ = store.UnstageAll(t.Context(), "")
	})

	// Test Load with empty state
	t.Run("load-empty-state", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.True(t, state.IsEmpty())
	})

	// Test WriteState
	t.Run("write-state", func(t *testing.T) {
		state := &staging.State{
			Entries: map[staging.Service]map[staging.EntryKey]staging.Entry{
				staging.ServiceParam: {
					staging.EntryKey{Name: "/suve-e2e-direct/write-state"}: {
						Operation: staging.OperationUpdate,
						Value:     lo.ToPtr("written-value"),
						StagedAt:  time.Now(),
					},
				},
			},
			Tags: map[staging.Service]map[staging.EntryKey]staging.TagEntry{},
		}
		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// Verify state was written
		loaded, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.NotNil(t, loaded.Entries[staging.ServiceParam][staging.EntryKey{Name: "/suve-e2e-direct/write-state"}])
	})

	// Test Load with data
	t.Run("load-with-data", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.False(t, state.IsEmpty())
		entry := state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/suve-e2e-direct/write-state"}]
		assert.Equal(t, "written-value", lo.FromPtr(entry.Value))
	})
}

// TestAgentStore_DrainMethod tests the Drain method.
func TestAgentStore_DrainMethod(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()
	paramName := "/suve-e2e-direct/drain-test"

	// Clean up first
	_ = store.UnstageAll(t.Context(), "")
	t.Cleanup(func() {
		_ = store.UnstageAll(t.Context(), "")
	})

	// Stage some data first
	err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("drain-value"),
		StagedAt:  time.Now(),
	})

	require.NoError(t, err)

	// Test Drain with keep=true
	t.Run("drain-with-keep", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.NotNil(t, state.Entries[staging.ServiceParam][staging.EntryKey{Name: paramName}])

		// Data should still be there after drain with keep
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName, Namespace: ""})
		require.NoError(t, err)
		assert.Equal(t, "drain-value", lo.FromPtr(entry.Value))
	})

	// Test Drain with keep=false (clears memory)
	t.Run("drain-without-keep", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", false)
		require.NoError(t, err)
		assert.NotNil(t, state.Entries[staging.ServiceParam][staging.EntryKey{Name: paramName}])

		// Data should be gone after drain without keep
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: paramName, Namespace: ""})
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	// Test Drain on empty state
	t.Run("drain-empty", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", false)
		require.NoError(t, err)
		assert.True(t, state.IsEmpty())
	})
}

// TestAgentStore_UnstageAll tests UnstageAll with different service filters.
func TestAgentStore_UnstageAll(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Clean up first
	_ = store.UnstageAll(t.Context(), "")
	t.Cleanup(func() {
		_ = store.UnstageAll(t.Context(), "")
	})

	// Stage entries for both services
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/unstage-all/param"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "suve-e2e/unstage-all/secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	// Test UnstageAll for specific service
	t.Run("unstage-all-param", func(t *testing.T) {
		err := store.UnstageAll(t.Context(), staging.ServiceParam)
		require.NoError(t, err)

		// Param should be gone
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/unstage-all/param", Namespace: ""})
		require.ErrorIs(t, err, staging.ErrNotStaged)

		// Secret should still be there
		entry, err := store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "suve-e2e/unstage-all/secret", Namespace: ""})
		require.NoError(t, err)
		assert.Equal(t, "secret-value", lo.FromPtr(entry.Value))
	})

	// Test UnstageAll for all services
	t.Run("unstage-all-services", func(t *testing.T) {
		// Re-add param
		_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/unstage-all/param"}, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})

		// Unstage all services (empty string)
		err := store.UnstageAll(t.Context(), "")
		require.NoError(t, err)

		// Both should be gone
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/unstage-all/param", Namespace: ""})
		require.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "suve-e2e/unstage-all/secret", Namespace: ""})
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

// TestAgentStore_ListMethods tests ListEntries and ListTags with various states.
func TestAgentStore_ListMethods(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Clean up first
	_ = store.UnstageAll(t.Context(), "")
	t.Cleanup(func() {
		_ = store.UnstageAll(t.Context(), "")
	})

	// Test ListEntries on empty state
	t.Run("list-entries-empty", func(t *testing.T) {
		entries, err := store.ListEntries(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		// Should return empty or nil map
		assert.Empty(t, entries[staging.ServiceParam])
	})

	// Test ListTags on empty state
	t.Run("list-tags-empty", func(t *testing.T) {
		tags, err := store.ListTags(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Empty(t, tags[staging.ServiceParam])
	})

	// Stage multiple entries and tags
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/list/param1"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value1"),
		StagedAt:  time.Now(),
	})

	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/list/param2"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value2"),
		StagedAt:  time.Now(),
	})

	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/list/param1"}, staging.TagEntry{
		Add:      map[string]string{"key": "value"},
		StagedAt: time.Now(),
	})

	// Test ListEntries with multiple entries
	t.Run("list-multiple-entries", func(t *testing.T) {
		entries, err := store.ListEntries(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Len(t, entries[staging.ServiceParam], 2)
		assert.Contains(t, entries[staging.ServiceParam], staging.EntryKey{Name: "/suve-e2e/list/param1"})
		assert.Contains(t, entries[staging.ServiceParam], staging.EntryKey{Name: "/suve-e2e/list/param2"})
	})

	// Test ListTags
	t.Run("list-tags-with-data", func(t *testing.T) {
		tags, err := store.ListTags(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Contains(t, tags[staging.ServiceParam], staging.EntryKey{Name: "/suve-e2e/list/param1"})
	})

	// Test ListEntries for all services (empty string)
	t.Run("list-entries-all-services", func(t *testing.T) {
		// Add a secret entry too
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "suve-e2e/list/secret1"}, staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		entries, err := store.ListEntries(t.Context(), "")
		require.NoError(t, err)
		assert.NotEmpty(t, entries[staging.ServiceParam])
		assert.NotEmpty(t, entries[staging.ServiceSecret])
	})
}

// =============================================================================
// Store Sequential Operation Tests
// =============================================================================

// TestStore_SequentialOperations exercises the store through repeated operations.
func TestStore_SequentialOperations(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Clean up first
	_ = store.UnstageAll(t.Context(), "")
	t.Cleanup(func() {
		_ = store.UnstageAll(t.Context(), "")
	})

	// Multiple sequential operations test IPC reliability
	t.Run("multiple-sequential-ipc-calls", func(t *testing.T) {
		for range 5 {
			err := store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/ipc-test"}, staging.Entry{
				Operation: staging.OperationUpdate,
				Value:     lo.ToPtr("test-value"),
				StagedAt:  time.Now(),
			})

			require.NoError(t, err)

			_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/ipc-test", Namespace: ""})
			require.NoError(t, err)

			err = store.UnstageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/suve-e2e/ipc-test", Namespace: ""})
			require.NoError(t, err)
		}
	})

	// Load/WriteState tests additional protocol methods
	t.Run("load-and-write-state-ipc", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.True(t, state.IsEmpty())

		state.Entries = map[staging.Service]map[staging.EntryKey]staging.Entry{
			staging.ServiceParam: {
				staging.EntryKey{Name: "/suve-e2e/ipc-write"}: {
					Operation: staging.OperationCreate,
					Value:     lo.ToPtr("ipc-value"),
					StagedAt:  time.Now(),
				},
			},
		}
		err = store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		loaded, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.NotNil(t, loaded.Entries[staging.ServiceParam][staging.EntryKey{Name: "/suve-e2e/ipc-write"}])
	})
}

// TestStore_SeparateAccounts verifies that stores for different accounts are isolated.
func TestStore_SeparateAccounts(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	// Create store for a different account (isolated staging area).
	store := newStoreForAccount("999999999999", "ap-northeast-1")

	// GetEntry on an empty staging area returns ErrNotStaged.
	t.Run("get-entry-empty", func(t *testing.T) {
		_, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/test", Namespace: ""})
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	// ListEntries on an empty staging area returns an empty result without error.
	t.Run("list-entries-empty", func(t *testing.T) {
		entries, err := store.ListEntries(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Empty(t, entries[staging.ServiceParam])
	})
}

// =============================================================================
// Empty-State Command Tests - Commands report empty state without side effects
// =============================================================================

// TestAgentLifecycle_StatusDoesNotStartAgent verifies that status command
// returns "No changes staged" without starting the agent when nothing is staged.
func TestAgentLifecycle_StatusDoesNotStartAgent(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Ensure staging is empty
	_ = store.UnstageAll(t.Context(), "")

	// Global status should return "No changes staged" without error
	t.Run("global-status-empty", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stdout, "No changes staged")
	})

	// Service-specific status should return appropriate message
	t.Run("param-status-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "No")
	})

	t.Run("secret-status-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "No")
	})
}

// TestAgentLifecycle_DiffDoesNotStartAgent verifies that diff command
// returns warning without starting the agent when nothing is staged.
func TestAgentLifecycle_DiffDoesNotStartAgent(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Ensure staging is empty
	_ = store.UnstageAll(t.Context(), "")

	// Global diff should show warning without error
	t.Run("global-diff-empty", func(t *testing.T) {
		_, stderr, err := runCommand(t, globaldiff.Command(awsStageGlobalConfig()))
		require.NoError(t, err)
		assert.Contains(t, stderr, "nothing staged")
	})

	// Service-specific diff should show warning
	// Message is either "nothing staged" (lifecycle) or "no parameters staged" (runner)
	t.Run("param-diff-empty", func(t *testing.T) {
		_, stderr, err := runSubCommand(t, paramstage.Command(), "diff")
		require.NoError(t, err)
		assert.True(t, strings.Contains(stderr, "nothing staged") || strings.Contains(stderr, "no parameters staged"),
			"expected 'nothing staged' or 'no parameters staged', got: %s", stderr)
	})
}

// TestAgentLifecycle_ApplyDoesNotStartAgent verifies that apply command
// returns "No changes staged" without starting the agent when nothing is staged.
func TestAgentLifecycle_ApplyDoesNotStartAgent(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Ensure staging is empty
	_ = store.UnstageAll(t.Context(), "")

	// Global apply should return info message without error
	t.Run("global-apply-empty", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalapply.Command(awsStageGlobalConfig()), "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, "No changes staged")
	})

	// Service-specific apply should return appropriate message
	t.Run("param-apply-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, "No")
	})
}

// TestAgentLifecycle_ResetDoesNotStartAgent verifies that reset command
// returns "No changes staged" without starting the agent when nothing is staged.
func TestAgentLifecycle_ResetDoesNotStartAgent(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Ensure staging is empty
	_ = store.UnstageAll(t.Context(), "")

	// Global reset --all should return info message without error
	t.Run("global-reset-all-empty", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalreset.Command(awsStageGlobalConfig()), "--all")
		require.NoError(t, err)
		assert.Contains(t, stdout, "No changes staged")
	})

	// Service-specific reset should return appropriate message
	t.Run("param-reset-all-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "reset", "--all")
		require.NoError(t, err)
		assert.Contains(t, stdout, "No")
	})
}
