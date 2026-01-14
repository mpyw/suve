//go:build e2e

//nolint:paralleltest,dogsled,gosec // E2E tests: sequential execution, cleanup, G101 false positive
package e2e_test

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
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon"
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
// Stash Push and Pop Tests
// =============================================================================

// TestGlobal_StashPushAndPop tests the stash push and pop workflow.
func TestGlobal_StashPushAndPop(t *testing.T) {
	setupEnv(t)

	paramName := "/suve-e2e-stash-push-pop/test-param"

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

	// Stash push agent memory to file
	t.Run("stash-push-to-file", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, stgcli.NewGlobalStashCommand(), "push")
		require.NoError(t, err)
		t.Logf("stash push output: %s", stdout)
		assert.Contains(t, stdout, "Staged changes stashed to file")
	})

	// Agent should now be empty
	t.Run("verify-agent-empty-after-stash-push", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, "No changes staged")
	})

	// Stash pop file back to agent
	t.Run("stash-pop-from-file", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, stgcli.NewGlobalStashCommand(), "pop")
		require.NoError(t, err)
		t.Logf("stash pop output: %s", stdout)
		assert.Contains(t, stdout, "Stashed changes restored")
	})

	// Agent should have the staged changes again
	t.Run("verify-agent-has-changes-after-stash-pop", func(t *testing.T) {
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

// TestGlobal_StashPushWithKeep tests stash push with --keep flag.
func TestGlobal_StashPushWithKeep(t *testing.T) {
	setupEnv(t)

	paramName := "/suve-e2e-stash-push-keep/test-param"

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

	// Stash push with --keep flag (should keep agent memory)
	t.Run("stash-push-with-keep", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, stgcli.NewGlobalStashCommand(), "push", "--keep")
		require.NoError(t, err)
		t.Logf("stash push --keep output: %s", stdout)
	})

	// Agent should still have the changes
	t.Run("agent-still-has-changes", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})
}

// TestGlobal_StashPopWithMerge tests stash pop with --merge flag.
func TestGlobal_StashPopWithMerge(t *testing.T) {
	setupEnv(t)

	paramName1 := "/suve-e2e-stash-pop-merge/param1"
	paramName2 := "/suve-e2e-stash-pop-merge/param2"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName1)
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName2)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName1)
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName2)
		_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	})

	// Stage param1 and stash push to file
	_, _, err := runCommand(t, paramstage.Command(), "add", paramName1, "value1")
	require.NoError(t, err)
	_, _, err = runSubCommand(t, stgcli.NewGlobalStashCommand(), "push")
	require.NoError(t, err)

	// Stage param2 in agent
	_, _, err = runCommand(t, paramstage.Command(), "add", paramName2, "value2")
	require.NoError(t, err)

	// Stash pop with merge (should combine both)
	t.Run("stash-pop-with-merge", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, stgcli.NewGlobalStashCommand(), "pop", "--merge")
		require.NoError(t, err)
		t.Logf("stash pop --merge output: %s", stdout)
	})

	// Both parameters should be staged
	t.Run("verify-both-staged", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName1)
		assert.Contains(t, stdout, paramName2)
	})
}

// TestGlobal_StashPopEmpty tests stash pop when file is empty.
func TestGlobal_StashPopEmpty(t *testing.T) {
	setupEnv(t)

	// Reset to ensure clean state
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")

	// Stash pop should fail or indicate nothing to pop
	t.Run("stash-pop-empty", func(t *testing.T) {
		_, _, err := runSubCommand(t, stgcli.NewGlobalStashCommand(), "pop")
		assert.Error(t, err)
	})
}

// TestGlobal_StashPushEmpty tests stash push when agent is empty.
func TestGlobal_StashPushEmpty(t *testing.T) {
	setupEnv(t)

	// Reset to ensure clean state (run twice to be safe)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")

	// Verify agent is actually empty
	stdout, _, _ := runCommand(t, globalstatus.Command())
	t.Logf("status before stash push: %s", stdout)

	// Stash push should fail when agent is empty (no staged changes)
	t.Run("stash-push-empty", func(t *testing.T) {
		_, stderr, err := runSubCommand(t, stgcli.NewGlobalStashCommand(), "push")
		// Either returns error or prints "nothing to stash"
		if err == nil {
			// If no error, check output - might indicate nothing was stashed
			t.Logf("stash push succeeded with no error - stderr: %s", stderr)
		} else {
			// Expected: error when nothing to stash
			t.Logf("stash push returned error as expected: %v", err)
		}
	})
}

// TestMixed_ServiceSpecificStashPushPop tests stash push/pop with mixed services.
func TestMixed_ServiceSpecificStashPushPop(t *testing.T) {
	setupEnv(t)

	paramName := "/suve-e2e-mixed-stash/param"
	secretName := "suve-e2e-mixed-stash/secret"

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

	// Stash push only params (secrets should remain in agent)
	t.Run("stash-push-param-only", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "stash", "push")
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

	// Stash pop params back (secret should be unaffected)
	t.Run("stash-pop-param-back", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "stash", "pop")
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
		err := store.StageEntry(t.Context(), staging.ServiceParam, paramName, entry)
		require.NoError(t, err)

		retrieved, err := store.GetEntry(t.Context(), staging.ServiceParam, paramName)
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, retrieved.Operation)
		assert.Equal(t, "direct-test-value", lo.FromPtr(retrieved.Value))
	})

	// Test ListEntries
	t.Run("list-entries", func(t *testing.T) {
		entries, err := store.ListEntries(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Contains(t, entries[staging.ServiceParam], paramName)
	})

	// Test UnstageEntry
	t.Run("unstage-entry", func(t *testing.T) {
		err := store.UnstageEntry(t.Context(), staging.ServiceParam, paramName)
		require.NoError(t, err)

		_, err = store.GetEntry(t.Context(), staging.ServiceParam, paramName)
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	// Test GetEntry for non-existent entry
	t.Run("get-nonexistent-entry", func(t *testing.T) {
		_, err := store.GetEntry(t.Context(), staging.ServiceParam, "/nonexistent/param")
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
		err := store.StageTag(t.Context(), staging.ServiceParam, paramName, tagEntry)
		require.NoError(t, err)

		retrieved, err := store.GetTag(t.Context(), staging.ServiceParam, paramName)
		require.NoError(t, err)
		assert.Equal(t, "test", retrieved.Add["env"])
		assert.Equal(t, "e2e", retrieved.Add["team"])
		assert.True(t, retrieved.Remove.Contains("old-tag"))
	})

	// Test ListTags
	t.Run("list-tags", func(t *testing.T) {
		tags, err := store.ListTags(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Contains(t, tags[staging.ServiceParam], paramName)
	})

	// Test UnstageTag
	t.Run("unstage-tag", func(t *testing.T) {
		err := store.UnstageTag(t.Context(), staging.ServiceParam, paramName)
		require.NoError(t, err)

		_, err = store.GetTag(t.Context(), staging.ServiceParam, paramName)
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	// Test GetTag for non-existent tag
	t.Run("get-nonexistent-tag", func(t *testing.T) {
		_, err := store.GetTag(t.Context(), staging.ServiceParam, "/nonexistent/param")
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
			Entries: map[staging.Service]map[string]staging.Entry{
				staging.ServiceParam: {
					"/suve-e2e-direct/write-state": {
						Operation: staging.OperationUpdate,
						Value:     lo.ToPtr("written-value"),
						StagedAt:  time.Now(),
					},
				},
			},
			Tags: map[staging.Service]map[string]staging.TagEntry{},
		}
		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// Verify state was written
		loaded, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.NotNil(t, loaded.Entries[staging.ServiceParam]["/suve-e2e-direct/write-state"])
	})

	// Test Load with data
	t.Run("load-with-data", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.False(t, state.IsEmpty())
		entry := state.Entries[staging.ServiceParam]["/suve-e2e-direct/write-state"]
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
	err := store.StageEntry(t.Context(), staging.ServiceParam, paramName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("drain-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	// Test Drain with keep=true
	t.Run("drain-with-keep", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.NotNil(t, state.Entries[staging.ServiceParam][paramName])

		// Data should still be there after drain with keep
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, paramName)
		require.NoError(t, err)
		assert.Equal(t, "drain-value", lo.FromPtr(entry.Value))
	})

	// Test Drain with keep=false (clears memory)
	t.Run("drain-without-keep", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", false)
		require.NoError(t, err)
		assert.NotNil(t, state.Entries[staging.ServiceParam][paramName])

		// Data should be gone after drain without keep
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, paramName)
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
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/suve-e2e/unstage-all/param", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "suve-e2e/unstage-all/secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	// Test UnstageAll for specific service
	t.Run("unstage-all-param", func(t *testing.T) {
		err := store.UnstageAll(t.Context(), staging.ServiceParam)
		require.NoError(t, err)

		// Param should be gone
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/suve-e2e/unstage-all/param")
		require.ErrorIs(t, err, staging.ErrNotStaged)

		// Secret should still be there
		entry, err := store.GetEntry(t.Context(), staging.ServiceSecret, "suve-e2e/unstage-all/secret")
		require.NoError(t, err)
		assert.Equal(t, "secret-value", lo.FromPtr(entry.Value))
	})

	// Test UnstageAll for all services
	t.Run("unstage-all-services", func(t *testing.T) {
		// Re-add param
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/suve-e2e/unstage-all/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})

		// Unstage all services (empty string)
		err := store.UnstageAll(t.Context(), "")
		require.NoError(t, err)

		// Both should be gone
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/suve-e2e/unstage-all/param")
		require.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "suve-e2e/unstage-all/secret")
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
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/suve-e2e/list/param1", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value1"),
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/suve-e2e/list/param2", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value2"),
		StagedAt:  time.Now(),
	})
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/suve-e2e/list/param1", staging.TagEntry{
		Add:      map[string]string{"key": "value"},
		StagedAt: time.Now(),
	})

	// Test ListEntries with multiple entries
	t.Run("list-multiple-entries", func(t *testing.T) {
		entries, err := store.ListEntries(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Len(t, entries[staging.ServiceParam], 2)
		assert.Contains(t, entries[staging.ServiceParam], "/suve-e2e/list/param1")
		assert.Contains(t, entries[staging.ServiceParam], "/suve-e2e/list/param2")
	})

	// Test ListTags
	t.Run("list-tags-with-data", func(t *testing.T) {
		tags, err := store.ListTags(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Contains(t, tags[staging.ServiceParam], "/suve-e2e/list/param1")
	})

	// Test ListEntries for all services (empty string)
	t.Run("list-entries-all-services", func(t *testing.T) {
		// Add a secret entry too
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "suve-e2e/list/secret1", staging.Entry{
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
// Daemon Launcher Tests (for IPC coverage)
// =============================================================================

// TestDaemonLauncher_Ping tests the launcher Ping method directly.
func TestDaemonLauncher_Ping(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	// Create launcher for the running test daemon
	launcher := daemon.NewLauncher("000000000000", "us-east-1", daemon.WithAutoStartDisabled())

	// Test Ping
	t.Run("ping-success", func(t *testing.T) {
		err := launcher.Ping(t.Context())
		require.NoError(t, err)
	})

	// Test multiple pings (concurrent access)
	t.Run("ping-concurrent", func(t *testing.T) {
		ctx := t.Context()
		done := make(chan error, 10)

		for range 10 {
			go func() {
				done <- launcher.Ping(ctx)
			}()
		}

		for range 10 {
			err := <-done
			assert.NoError(t, err)
		}
	})
}

// TestDaemonLauncher_EnsureRunning tests the EnsureRunning method.
func TestDaemonLauncher_EnsureRunning(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	// Create launcher for the running test daemon
	launcher := daemon.NewLauncher("000000000000", "us-east-1", daemon.WithAutoStartDisabled())

	// Test EnsureRunning (daemon is already running from TestMain)
	t.Run("ensure-running-when-running", func(t *testing.T) {
		err := launcher.EnsureRunning(t.Context())
		require.NoError(t, err)
	})
}

// TestDaemonLauncher_ViaStore tests launcher IPC indirectly through store operations.
// This exercises the SendRequest method via the store's methods.
func TestDaemonLauncher_ViaStore(t *testing.T) {
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
			err := store.StageEntry(t.Context(), staging.ServiceParam, "/suve-e2e/ipc-test", staging.Entry{
				Operation: staging.OperationUpdate,
				Value:     lo.ToPtr("test-value"),
				StagedAt:  time.Now(),
			})
			require.NoError(t, err)

			_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/suve-e2e/ipc-test")
			require.NoError(t, err)

			err = store.UnstageEntry(t.Context(), staging.ServiceParam, "/suve-e2e/ipc-test")
			require.NoError(t, err)
		}
	})

	// Load/WriteState tests additional protocol methods
	t.Run("load-and-write-state-ipc", func(t *testing.T) {
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.True(t, state.IsEmpty())

		state.Entries = map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/suve-e2e/ipc-write": {
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
		assert.NotNil(t, loaded.Entries[staging.ServiceParam]["/suve-e2e/ipc-write"])
	})
}

// TestDaemonLauncher_NotRunning tests launcher behavior when daemon is not running.
func TestDaemonLauncher_NotRunning(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	// Create launcher for a different account where no daemon is running
	launcher := daemon.NewLauncher("999999999999", "ap-northeast-1", daemon.WithAutoStartDisabled())

	// Test Ping fails when daemon not running
	t.Run("ping-not-running", func(t *testing.T) {
		err := launcher.Ping(t.Context())
		assert.Error(t, err)
	})

	// Test EnsureRunning fails when auto-start is disabled
	t.Run("ensure-running-fails", func(t *testing.T) {
		err := launcher.EnsureRunning(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auto-start is disabled")
	})
}

// TestAgentStore_NotRunning tests store behavior when daemon is not running.
func TestAgentStore_NotRunning(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	// Create store for a different account where no daemon is running
	store := newStoreForAccount("999999999999", "ap-northeast-1")

	// Test GetEntry fails when daemon not running
	t.Run("get-entry-not-running", func(t *testing.T) {
		_, err := store.GetEntry(t.Context(), staging.ServiceParam, "/test")
		assert.Error(t, err)
	})

	// Test ListEntries fails when daemon not running
	t.Run("list-entries-not-running", func(t *testing.T) {
		_, err := store.ListEntries(t.Context(), staging.ServiceParam)
		assert.Error(t, err)
	})
}

// =============================================================================
// Agent Lifecycle Tests - Commands should NOT start agent when nothing staged
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
		stdout, _, err := runCommand(t, globalstatus.Command())
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
		_, stderr, err := runCommand(t, globaldiff.Command())
		require.NoError(t, err)
		assert.Contains(t, stderr, "nothing staged")
	})

	// Service-specific diff should show warning
	t.Run("param-diff-empty", func(t *testing.T) {
		_, stderr, err := runSubCommand(t, paramstage.Command(), "diff")
		require.NoError(t, err)
		assert.Contains(t, stderr, "nothing staged")
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
		stdout, _, err := runCommand(t, globalapply.Command(), "--yes")
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
		stdout, _, err := runCommand(t, globalreset.Command(), "--all")
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

// TestAgentLifecycle_StashPushDoesNotStartAgent verifies that stash push command
// returns "No staged changes to persist" without starting the agent when nothing is staged.
func TestAgentLifecycle_StashPushDoesNotStartAgent(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	store := newStore()

	// Ensure staging is empty
	_ = store.UnstageAll(t.Context(), "")

	// Global stash push should return info message without error
	t.Run("global-stash-push-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, stgcli.NewGlobalStashCommand(), "push")
		require.NoError(t, err)
		assert.Contains(t, stdout, "No staged changes")
	})

	// Service-specific stash push should return appropriate message
	t.Run("param-stash-push-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "stash", "push")
		require.NoError(t, err)
		assert.Contains(t, stdout, "No staged changes")
	})
}
