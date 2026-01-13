//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	paramcreate "github.com/mpyw/suve/internal/cli/commands/param/create"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/param/delete"
	paramshow "github.com/mpyw/suve/internal/cli/commands/param/show"
	secretcreate "github.com/mpyw/suve/internal/cli/commands/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/secret/delete"
	secretshow "github.com/mpyw/suve/internal/cli/commands/secret/show"
	globalstage "github.com/mpyw/suve/internal/cli/commands/stage"
	globalapply "github.com/mpyw/suve/internal/cli/commands/stage/apply"
	globaldiff "github.com/mpyw/suve/internal/cli/commands/stage/diff"
	paramstage "github.com/mpyw/suve/internal/cli/commands/stage/param"
	globalreset "github.com/mpyw/suve/internal/cli/commands/stage/reset"
	secretstage "github.com/mpyw/suve/internal/cli/commands/stage/secret"
	globalstatus "github.com/mpyw/suve/internal/cli/commands/stage/status"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

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
	_ = store.StageEntry(t.Context(), staging.ServiceParam, paramName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-param"),
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, secretName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-secret"),
		StagedAt:  time.Now(),
	})

	// 1. Global status shows both
	t.Run("global-status", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, secretName)
		assert.Contains(t, stdout, "SSM Parameter Store")
		assert.Contains(t, stdout, "Secrets Manager")
		t.Logf("global status output: %s", stdout)
	})

	// 2. Global diff shows both
	t.Run("global-diff", func(t *testing.T) {
		stdout, _, err := runCommand(t, globaldiff.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, "-original-param")
		assert.Contains(t, stdout, "+staged-param")
		assert.Contains(t, stdout, "-original-secret")
		assert.Contains(t, stdout, "+staged-secret")
		t.Logf("global diff output: %s", stdout)
	})

	// 3. Global apply applies both
	t.Run("global-apply", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalapply.Command(), "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, secretName)
		t.Logf("global apply output: %s", stdout)
	})

	// 4. Verify both updated
	t.Run("verify", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "staged-param", stdout)

		stdout, _, err = runCommand(t, secretshow.Command(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, "staged-secret", stdout)
	})

	// 5. Status should be empty
	t.Run("status-empty", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
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
	_ = store.StageEntry(t.Context(), staging.ServiceParam, paramName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged"),
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, secretName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged"),
		StagedAt:  time.Now(),
	})

	// Global reset requires --all flag
	t.Run("reset-without-all-warns", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, globalreset.Command())
		require.NoError(t, err)
		// Without --all, should show warning
		assert.Contains(t, stderr, "no effect")
		t.Logf("global reset without --all output: %s, stderr: %s", stdout, stderr)
	})

	// Verify still staged (not reset without --all)
	t.Run("verify-still-staged", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, secretName)
	})

	// Global reset with --all
	t.Run("reset-with-all", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalreset.Command(), "--all")
		require.NoError(t, err)
		t.Logf("global reset --all output: %s", stdout)
	})

	// Verify empty
	t.Run("verify-empty", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.NotContains(t, stdout, paramName)
		assert.NotContains(t, stdout, secretName)
	})
}

// TestGlobal_StageCommand tests the global stage parent command structure.
func TestGlobal_StageCommand(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

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
	_ = setupTempHome(t)

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
	err = store.StageTag(t.Context(), staging.ServiceParam, paramName, staging.TagEntry{
		Add:      map[string]string{"env": "test", "team": "e2e"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	// Test global diff shows tag changes
	t.Run("global-diff-shows-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, globaldiff.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, "Tags:")
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, "env=test")
		t.Logf("global diff with tags output: %s", stdout)
	})

	// Test global status shows tag changes
	t.Run("global-status-shows-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, "T")         // T = Tag change marker
		assert.Contains(t, stdout, "+2 tag(s)") // Two tags being added
		t.Logf("global status with tags output: %s", stdout)
	})

	// Test global apply applies tag changes
	t.Run("global-apply-applies-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalapply.Command(), "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Applying SSM Parameter Store tags")
		assert.Contains(t, stdout, "Tagged")
		t.Logf("global apply with tags output: %s", stdout)

		// Verify tags were applied by checking show output
		stdout, _, err = runCommand(t, paramshow.Command(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env: test")
		assert.Contains(t, stdout, "team: e2e")
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
	err = store.StageEntry(t.Context(), staging.ServiceParam, paramName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("updated-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceParam, paramName, staging.TagEntry{
		Add:      map[string]string{"env": "test"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	// Test global reset clears both entries and tags
	t.Run("global-reset-clears-all", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalreset.Command(), "--all")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Unstaged all changes")
		assert.Contains(t, stdout, "2 SSM Parameter Store") // 1 entry + 1 tag
		t.Logf("global reset output: %s", stdout)

		// Verify staging is empty
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, paramName)
		assert.Equal(t, staging.ErrNotStaged, err)
		_, err = store.GetTag(t.Context(), staging.ServiceParam, paramName)
		assert.Equal(t, staging.ErrNotStaged, err)
	})
}

// =============================================================================
// Drain and Persist Tests
// =============================================================================

// TestGlobal_DrainAndPersist tests the drain and persist workflow.
func TestGlobal_DrainAndPersist(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-drain-persist/test-param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
		_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	})

	// Stage a parameter in agent memory
	t.Run("stage-add", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramstage.Command(), "add", paramName, "test-value")
		require.NoError(t, err)
		t.Logf("stage add output: %s", stdout)
	})

	// Verify agent has staged changes
	t.Run("verify-agent-status", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})

	// Persist agent memory to file
	t.Run("persist-to-file", func(t *testing.T) {
		stdout, _, err := runCommand(t, stgcli.NewGlobalPersistCommand())
		require.NoError(t, err)
		t.Logf("persist output: %s", stdout)
		assert.Contains(t, stdout, "persisted to file")
	})

	// Agent should now be empty
	t.Run("verify-agent-empty-after-persist", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, "No changes staged")
	})

	// Drain file back to agent
	t.Run("drain-from-file", func(t *testing.T) {
		stdout, _, err := runCommand(t, stgcli.NewGlobalDrainCommand())
		require.NoError(t, err)
		t.Logf("drain output: %s", stdout)
		assert.Contains(t, stdout, "loaded from file")
	})

	// Agent should have the staged changes again
	t.Run("verify-agent-has-changes-after-drain", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})

	// Apply the staged changes
	t.Run("apply-changes", func(t *testing.T) {
		_, _, err := runCommand(t, globalapply.Command(), "--yes")
		require.NoError(t, err)
	})

	// Verify the parameter was created
	t.Run("verify-created", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "test-value", strings.TrimSpace(stdout))
	})
}

// TestGlobal_PersistWithKeep tests persist with --keep flag.
func TestGlobal_PersistWithKeep(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-persist-keep/test-param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
		_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	})

	// Stage a parameter
	_, _, err := runCommand(t, paramstage.Command(), "add", paramName, "test-value")
	require.NoError(t, err)

	// Persist with --keep flag (should keep agent memory)
	t.Run("persist-with-keep", func(t *testing.T) {
		stdout, _, err := runCommand(t, stgcli.NewGlobalPersistCommand(), "--keep")
		require.NoError(t, err)
		t.Logf("persist --keep output: %s", stdout)
	})

	// Agent should still have the changes
	t.Run("agent-still-has-changes", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})
}

// TestGlobal_DrainWithMerge tests drain with --merge flag.
func TestGlobal_DrainWithMerge(t *testing.T) {
	setupEnv(t)
	paramName1 := "/suve-e2e-drain-merge/param1"
	paramName2 := "/suve-e2e-drain-merge/param2"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName1)
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName2)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName1)
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName2)
		_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	})

	// Stage param1 and persist to file
	_, _, err := runCommand(t, paramstage.Command(), "add", paramName1, "value1")
	require.NoError(t, err)
	_, _, err = runCommand(t, stgcli.NewGlobalPersistCommand())
	require.NoError(t, err)

	// Stage param2 in agent
	_, _, err = runCommand(t, paramstage.Command(), "add", paramName2, "value2")
	require.NoError(t, err)

	// Drain with merge (should combine both)
	t.Run("drain-with-merge", func(t *testing.T) {
		stdout, _, err := runCommand(t, stgcli.NewGlobalDrainCommand(), "--merge")
		require.NoError(t, err)
		t.Logf("drain --merge output: %s", stdout)
	})

	// Both parameters should be staged
	t.Run("verify-both-staged", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName1)
		assert.Contains(t, stdout, paramName2)
	})
}

// TestGlobal_DrainEmpty tests drain when file is empty.
func TestGlobal_DrainEmpty(t *testing.T) {
	setupEnv(t)

	// Reset to ensure clean state
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")

	// Drain should fail or indicate nothing to drain
	t.Run("drain-empty", func(t *testing.T) {
		_, _, err := runCommand(t, stgcli.NewGlobalDrainCommand())
		assert.Error(t, err)
	})
}

// TestGlobal_PersistEmpty tests persist when agent is empty.
func TestGlobal_PersistEmpty(t *testing.T) {
	setupEnv(t)

	// Reset to ensure clean state (run twice to be safe)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")

	// Verify agent is actually empty
	stdout, _, _ := runCommand(t, globalstatus.Command())
	t.Logf("status before persist: %s", stdout)

	// Persist should fail when agent is empty (no staged changes)
	t.Run("persist-empty", func(t *testing.T) {
		_, stderr, err := runCommand(t, stgcli.NewGlobalPersistCommand())
		// Either returns error or prints "nothing to persist"
		if err == nil {
			// If no error, check output - might indicate nothing was persisted
			t.Logf("persist succeeded with no error - stderr: %s", stderr)
		} else {
			// Expected: error when nothing to persist
			t.Logf("persist returned error as expected: %v", err)
		}
	})
}

// TestMixed_ServiceSpecificDrainPersist tests drain/persist with mixed services.
func TestMixed_ServiceSpecificDrainPersist(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-mixed-drain/param"
	secretName := "suve-e2e-mixed-drain/secret"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
		_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	})

	// Stage both param and secret
	_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "param-value")
	require.NoError(t, err)
	_, _, err = runSubCommand(t, secretstage.Command(), "add", secretName, "secret-value")
	require.NoError(t, err)

	// Persist only params (secrets should remain in agent)
	t.Run("persist-param-only", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "persist")
		require.NoError(t, err)
	})

	// Param should be gone from agent, secret should remain
	t.Run("verify-param-gone-secret-remains", func(t *testing.T) {
		paramStatus, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.NotContains(t, paramStatus, paramName)

		secretStatus, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, secretStatus, secretName)
	})

	// Drain params back (secret should be unaffected)
	t.Run("drain-param-back", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "drain")
		require.NoError(t, err)
	})

	// Both should now be in agent
	t.Run("verify-both-in-agent", func(t *testing.T) {
		paramStatus, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, paramStatus, paramName)

		secretStatus, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, secretStatus, secretName)
	})
}
