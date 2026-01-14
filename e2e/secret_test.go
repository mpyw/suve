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

	secretcreate "github.com/mpyw/suve/internal/cli/commands/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/secret/delete"
	secretdiff "github.com/mpyw/suve/internal/cli/commands/secret/diff"
	secretlist "github.com/mpyw/suve/internal/cli/commands/secret/list"
	secretlog "github.com/mpyw/suve/internal/cli/commands/secret/log"
	secretrestore "github.com/mpyw/suve/internal/cli/commands/secret/restore"
	secretshow "github.com/mpyw/suve/internal/cli/commands/secret/show"
	secrettag "github.com/mpyw/suve/internal/cli/commands/secret/tag"
	secretuntag "github.com/mpyw/suve/internal/cli/commands/secret/untag"
	secretupdate "github.com/mpyw/suve/internal/cli/commands/secret/update"
	globalreset "github.com/mpyw/suve/internal/cli/commands/stage/reset"
	secretstage "github.com/mpyw/suve/internal/cli/commands/stage/secret"
	"github.com/mpyw/suve/internal/staging"
)

// =============================================================================
// Secrets Manager Basic Commands Tests
// =============================================================================

// TestSecret_FullWorkflow tests the complete Secrets Manager workflow.
func TestSecret_FullWorkflow(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-test/basic/secret"

	// Cleanup: force delete secret if it exists
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// 1. Create secret
	t.Run("create", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretcreate.Command(), secretName, "initial-secret")
		require.NoError(t, err)
		t.Logf("create output: %s", stdout)
	})

	// 2. Show secret
	t.Run("show", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretshow.Command(), secretName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "initial-secret")
		t.Logf("show output: %s", stdout)
	})

	// 3. Cat secret
	t.Run("cat", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretshow.Command(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, "initial-secret", stdout)
	})

	// 4. Update secret (with -y to skip confirmation)
	t.Run("update", func(t *testing.T) {
		_, _, err := runCommand(t, secretupdate.Command(), "--yes", secretName, "updated-secret")
		require.NoError(t, err)
	})

	// 5. Log
	t.Run("log", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretlog.Command(), secretName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Version")
		t.Logf("log output: %s", stdout)
	})

	// 6. Log with options
	t.Run("log-with-options", func(t *testing.T) {
		// --oneline
		stdout, _, err := runCommand(t, secretlog.Command(), "--oneline", secretName)
		require.NoError(t, err)
		t.Logf("log --oneline output: %s", stdout)

		// -p (patch) - log shows from newest to oldest, so diff is current→previous
		stdout, _, err = runCommand(t, secretlog.Command(), "-p", secretName)
		require.NoError(t, err)
		// Check that diff contains both values (order depends on log direction)
		assert.Contains(t, stdout, "initial-secret")
		assert.Contains(t, stdout, "updated-secret")
		t.Logf("log -p output: %s", stdout)
	})

	// 7. Diff - Compare AWSPREVIOUS with AWSCURRENT
	t.Run("diff", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretdiff.Command(), secretName+":AWSPREVIOUS", secretName+":AWSCURRENT")
		require.NoError(t, err)
		assert.Contains(t, stdout, "-initial-secret")
		assert.Contains(t, stdout, "+updated-secret")
		t.Logf("diff output: %s", stdout)
	})

	// 8. Diff with single arg
	t.Run("diff-single-arg", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretdiff.Command(), secretName+":AWSPREVIOUS")
		require.NoError(t, err)
		assert.Contains(t, stdout, "-initial-secret")
		assert.Contains(t, stdout, "+updated-secret")
	})

	// 9. Diff with ~SHIFT
	// Note: Secrets Manager shift (~) may not work correctly in localstack due to version history limitations
	t.Run("diff-shift", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, secretdiff.Command(), secretName+"~1")
		t.Logf("diff-shift stdout: %s", stdout)
		t.Logf("diff-shift stderr: %s", stderr)
		// Skip strict assertion - localstack may not support shift properly
		if err == nil && stdout != "" {
			// If it works, check for the values
			assert.True(t, strings.Contains(stdout, "initial-secret") || strings.Contains(stdout, "updated-secret"))
		}
	})

	// 10. List
	t.Run("list", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretlist.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, secretName)
		t.Logf("list output: %s", stdout)
	})

	// 11. Delete with recovery window
	t.Run("delete-with-recovery", func(t *testing.T) {
		_, _, err := runCommand(t, secretdelete.Command(), "--yes", "--recovery-window", "7", secretName)
		require.NoError(t, err)
	})

	// 12. Restore
	t.Run("restore", func(t *testing.T) {
		_, _, err := runCommand(t, secretrestore.Command(), secretName)
		require.NoError(t, err)
	})

	// 13. Verify restored
	t.Run("verify-restored", func(t *testing.T) {
		_, _, err := runCommand(t, secretshow.Command(), secretName)
		require.NoError(t, err)
	})

	// 14. Force delete
	t.Run("force-delete", func(t *testing.T) {
		_, _, err := runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
		require.NoError(t, err)
	})

	// 15. Verify deleted
	t.Run("verify-deleted", func(t *testing.T) {
		_, _, err := runCommand(t, secretshow.Command(), secretName)
		assert.Error(t, err, "expected error after deletion")
	})
}

// TestSecret_VersionSpecifiers tests Secrets Manager version specifier syntax.
func TestSecret_VersionSpecifiers(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-test/version/secret"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create with multiple versions
	_, _, err := runCommand(t, secretcreate.Command(), secretName, "v1")
	require.NoError(t, err)
	_, _, err = runCommand(t, secretupdate.Command(), "--yes", secretName, "v2")
	require.NoError(t, err)
	_, _, err = runCommand(t, secretupdate.Command(), "--yes", secretName, "v3")
	require.NoError(t, err)

	// Test :LABEL
	t.Run("label", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretshow.Command(), "--raw", secretName+":AWSCURRENT")
		require.NoError(t, err)
		assert.Equal(t, "v3", stdout)

		stdout, _, err = runCommand(t, secretshow.Command(), "--raw", secretName+":AWSPREVIOUS")
		require.NoError(t, err)
		assert.Equal(t, "v2", stdout)
	})

	// Test ~SHIFT
	// Note: Secrets Manager shift (~) may not work correctly in localstack due to version history limitations
	t.Run("shift", func(t *testing.T) {
		// ~1 = 1 version ago
		stdout, _, err := runCommand(t, secretshow.Command(), "--raw", secretName+"~1")
		// Localstack may not support shift properly, skip strict assertion
		t.Logf("shift ~1 stdout: %s, err: %v", stdout, err)

		if err == nil {
			// If it works, value should be one of the previous versions
			assert.True(t, stdout == "v1" || stdout == "v2" || stdout == "v3",
				"expected v1, v2 or v3, got %s", stdout)
		}
	})

	// Test :LABEL~SHIFT combination
	// Note: May not work in localstack due to version history limitations
	t.Run("label-and-shift", func(t *testing.T) {
		// :AWSCURRENT~1 = 1 version before current
		stdout, _, err := runCommand(t, secretshow.Command(), "--raw", secretName+":AWSCURRENT~1")
		t.Logf("label-and-shift stdout: %s, err: %v", stdout, err)
		// Skip strict assertion - localstack may error with "version shift out of range"
		if err == nil {
			assert.True(t, stdout == "v1" || stdout == "v2",
				"expected v1 or v2, got %s", stdout)
		}
	})
}

// =============================================================================
// Secrets Manager Staging Workflow Tests
// =============================================================================

// TestSecret_StagingWorkflow tests the complete Secrets Manager staging workflow.
func TestSecret_StagingWorkflow(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	secretName := "suve-e2e-staging/workflow/secret"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// 1. Create initial secret
	t.Run("setup", func(t *testing.T) {
		_, _, err := runCommand(t, secretcreate.Command(), secretName, "original-secret")
		require.NoError(t, err)
	})

	// 2. Stage update
	t.Run("stage-update", func(t *testing.T) {
		store := newStore()
		err := store.StageEntry(t.Context(), staging.ServiceSecret, secretName, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("staged-secret"),
			StagedAt:  time.Now(),
		})
		require.NoError(t, err)
	})

	// 3. Status
	t.Run("status", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, secretName)
		assert.Contains(t, stdout, "M")
		t.Logf("status output: %s", stdout)
	})

	// 4. Diff
	t.Run("diff", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "diff", secretName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "-original-secret")
		assert.Contains(t, stdout, "+staged-secret")
	})

	// 5. Push
	t.Run("apply", func(t *testing.T) {
		_, _, err := runSubCommand(t, secretstage.Command(), "apply", "--yes")
		require.NoError(t, err)
	})

	// 6. Verify
	t.Run("verify", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretshow.Command(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, "staged-secret", stdout)
	})

	// 7. Stage delete with options
	t.Run("stage-delete-with-force", func(t *testing.T) {
		_, _, err := runSubCommand(t, secretstage.Command(), "delete", "--force", secretName)
		require.NoError(t, err)
	})

	// 8. Status shows delete
	t.Run("status-delete", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, secretName)
		assert.Contains(t, stdout, "D")
	})

	// 9. Push delete
	t.Run("apply-delete", func(t *testing.T) {
		_, _, err := runSubCommand(t, secretstage.Command(), "apply", "--yes")
		require.NoError(t, err)
	})

	// 10. Verify deleted
	t.Run("verify-deleted", func(t *testing.T) {
		_, _, err := runCommand(t, secretshow.Command(), secretName)
		assert.Error(t, err)
	})
}

// TestSecret_StagingDeleteOptions tests Secrets Manager staging with delete options.
func TestSecret_StagingDeleteOptions(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	secretName := "suve-e2e-staging/delete-opts/secret"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create secret
	_, _, _ = runCommand(t, secretcreate.Command(), secretName, "test-value")

	// Test delete with recovery window
	t.Run("delete-with-recovery-window", func(t *testing.T) {
		_, _, err := runSubCommand(t, secretstage.Command(), "delete", "--recovery-window", "14", secretName)
		require.NoError(t, err)

		// Verify options are stored
		store := newStore()
		entry, err := store.GetEntry(t.Context(), staging.ServiceSecret, secretName)
		require.NoError(t, err)
		require.NotNil(t, entry.DeleteOptions)
		assert.Equal(t, 14, entry.DeleteOptions.RecoveryWindow)
		assert.False(t, entry.DeleteOptions.Force)
	})
}

// TestSecret_ErrorCases tests various Secrets Manager error scenarios.
func TestSecret_ErrorCases(t *testing.T) {
	setupEnv(t)

	// Show non-existent secret
	t.Run("show-nonexistent", func(t *testing.T) {
		_, _, err := runCommand(t, secretshow.Command(), "nonexistent-secret-12345")
		assert.Error(t, err)
	})

	// Note: localstack may not error on delete of non-existent secret
	// So we skip this test for localstack compatibility

	// Invalid label
	t.Run("invalid-label", func(t *testing.T) {
		_, _, err := runCommand(t, secretshow.Command(), "secret:INVALIDLABEL")
		assert.Error(t, err)
	})

	// Invalid recovery window
	t.Run("invalid-recovery-window", func(t *testing.T) {
		_, _, err := runCommand(t, secretdelete.Command(), "--yes", "--recovery-window", "5", "some-secret")
		assert.Error(t, err) // Must be 7-30
	})
}

// TestSecret_SpecialCharactersInName tests secret names with special characters.
func TestSecret_SpecialCharactersInName(t *testing.T) {
	setupEnv(t)

	testCases := []struct {
		name       string
		secretName string
	}{
		{"with-slash", "suve-e2e/special/name"},
		{"with-hyphen", "suve-e2e-special-name"},
		{"with-underscore", "suve_e2e_special_name"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Cleanup
			_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", tc.secretName)
			t.Cleanup(func() {
				_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", tc.secretName)
			})

			_, _, err := runCommand(t, secretcreate.Command(), tc.secretName, "test-value")
			require.NoError(t, err)

			stdout, _, err := runCommand(t, secretshow.Command(), "--raw", tc.secretName)
			require.NoError(t, err)
			assert.Equal(t, "test-value", stdout)
		})
	}
}

// TestSecret_StagingAddViaCLI tests the Secrets Manager stage add command via CLI.
func TestSecret_StagingAddViaCLI(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	secretName := "suve-e2e-staging/add-cli/secret"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// 1. Stage add via CLI
	t.Run("add-via-cli", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "add", secretName, "cli-staged-secret")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
		t.Logf("stage add output: %s", stdout)
	})

	// 2. Verify status
	t.Run("status", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, secretName)
		assert.Contains(t, stdout, "A")
	})

	// 3. Push to create
	t.Run("apply", func(t *testing.T) {
		_, _, err := runSubCommand(t, secretstage.Command(), "apply", "--yes")
		require.NoError(t, err)
	})

	// 4. Verify created
	t.Run("verify", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretshow.Command(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, "cli-staged-secret", stdout)
	})
}

// TestSecret_StagingAddExistingResourceFails tests that adding an existing secret fails.
func TestSecret_StagingAddExistingResourceFails(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	secretName := "suve-e2e-staging/add-existing/secret"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create the secret first
	_, _, err := runCommand(t, secretcreate.Command(), secretName, "existing-value")
	require.NoError(t, err)

	// Try to stage add - should fail because resource already exists
	t.Run("add-existing-fails", func(t *testing.T) {
		_, _, err := runSubCommand(t, secretstage.Command(), "add", secretName, "new-value")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		t.Logf("expected error: %v", err)
	})
}

// TestSecret_TagAndUntag tests the secret tag and untag commands.
func TestSecret_TagAndUntag(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-tag/test-secret"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create secret first
	t.Run("create", func(t *testing.T) {
		_, _, err := runCommand(t, secretcreate.Command(), secretName, "test-value")
		require.NoError(t, err)
	})

	// Add tags
	t.Run("tag", func(t *testing.T) {
		stdout, _, err := runCommand(t, secrettag.Command(), secretName, "env=test", "team=suve")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Tagged")
		t.Logf("tag output: %s", stdout)
	})

	// Verify tags are added
	t.Run("verify-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretshow.Command(), secretName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env: test")
		assert.Contains(t, stdout, "team: suve")
	})

	// Remove one tag
	t.Run("untag", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretuntag.Command(), secretName, "team")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Untagged")
		t.Logf("untag output: %s", stdout)
	})

	// Verify tag is removed
	t.Run("verify-untag", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretshow.Command(), secretName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env: test")
		assert.NotContains(t, stdout, "team: suve")
	})
}

// TestSecret_TagInvalidFormat tests error handling for invalid tag formats.
func TestSecret_TagInvalidFormat(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-tag/invalid-format"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create secret first
	_, _, err := runCommand(t, secretcreate.Command(), secretName, "test-value")
	require.NoError(t, err)

	// Try to add invalid tag format
	t.Run("invalid-format", func(t *testing.T) {
		_, _, err := runCommand(t, secrettag.Command(), secretName, "invalid-tag-no-equals")
		require.Error(t, err)
		t.Logf("expected error: %v", err)
	})
}

// TestSecret_TagNonExistent tests tagging a non-existent secret.
func TestSecret_TagNonExistent(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-tag/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)

	// Try to tag non-existent secret
	_, _, err := runCommand(t, secrettag.Command(), secretName, "env=test")
	require.Error(t, err)
	t.Logf("expected error: %v", err)
}

// TestSecret_UntagNonExistent tests untagging a non-existent secret.
func TestSecret_UntagNonExistent(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-untag/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)

	// Try to untag non-existent secret
	_, _, err := runCommand(t, secretuntag.Command(), secretName, "env")
	require.Error(t, err)
	t.Logf("expected error: %v", err)
}

// TestSecret_UpdateNonExistent tests updating a non-existent secret.
func TestSecret_UpdateNonExistent(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-update/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)

	// Try to update non-existent secret
	_, _, err := runCommand(t, secretupdate.Command(), "--yes", secretName, "new-value")
	assert.Error(t, err)
}

// TestSecret_UpdateMissingArgs tests error handling for missing arguments.
func TestSecret_UpdateMissingArgs(t *testing.T) {
	setupEnv(t)

	// No arguments at all
	t.Run("no-args", func(t *testing.T) {
		_, _, err := runCommand(t, secretupdate.Command())
		assert.Error(t, err)
	})

	// Only name, no value
	t.Run("no-value", func(t *testing.T) {
		_, _, err := runCommand(t, secretupdate.Command(), "test/secret")
		assert.Error(t, err)
	})
}

// TestSecret_LogNonExistent tests log for non-existent secret.
func TestSecret_LogNonExistent(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-log/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)

	// Try to get log
	_, _, err := runCommand(t, secretlog.Command(), secretName)
	assert.Error(t, err)
}

// TestSecret_DiffNonExistent tests diff for non-existent secret.
func TestSecret_DiffNonExistent(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-diff/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)

	// Try to diff
	_, _, err := runCommand(t, secretdiff.Command(), secretName)
	assert.Error(t, err)
}

// TestSecret_ShowRaw tests secret show with --raw flag.
func TestSecret_ShowRaw(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-show/raw-test"
	secretValue := "raw-secret-value"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create secret
	_, _, err := runCommand(t, secretcreate.Command(), secretName, secretValue)
	require.NoError(t, err)

	// Show raw
	t.Run("raw", func(t *testing.T) {
		stdout, _, err := runCommand(t, secretshow.Command(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, secretValue, strings.TrimSuffix(stdout, "\n"))
	})
}

// TestSecret_ShowNonExistent tests show for non-existent secret.
func TestSecret_ShowNonExistent(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-show/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)

	// Try to show
	_, _, err := runCommand(t, secretshow.Command(), secretName)
	assert.Error(t, err)
}

// TestSecret_CreateAndTag tests creating a secret and adding tags after.
func TestSecret_CreateAndTag(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-create/and-tag"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create secret
	stdout, _, err := runCommand(t, secretcreate.Command(), secretName, "value")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Created")

	// Add tags after creation
	_, _, err = runCommand(t, secrettag.Command(), secretName, "env=test", "team=suve")
	require.NoError(t, err)

	// Verify tags
	stdout, _, err = runCommand(t, secretshow.Command(), secretName)
	require.NoError(t, err)
	assert.Contains(t, stdout, "env: test")
	assert.Contains(t, stdout, "team: suve")
}

// TestSecret_CreateDuplicate tests creating a duplicate secret.
func TestSecret_CreateDuplicate(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-create/duplicate"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create first
	_, _, err := runCommand(t, secretcreate.Command(), secretName, "value")
	require.NoError(t, err)

	// Try to create again
	_, _, err = runCommand(t, secretcreate.Command(), secretName, "value2")
	assert.Error(t, err)
}

// TestSecret_CreateMissingArgs tests error handling for missing arguments.
func TestSecret_CreateMissingArgs(t *testing.T) {
	setupEnv(t)

	// No arguments at all
	t.Run("no-args", func(t *testing.T) {
		_, _, err := runCommand(t, secretcreate.Command())
		assert.Error(t, err)
	})

	// Only name, no value
	t.Run("no-value", func(t *testing.T) {
		_, _, err := runCommand(t, secretcreate.Command(), "test/secret")
		assert.Error(t, err)
	})
}

// TestSecret_DeleteNonExistent tests deleting a non-existent secret.
func TestSecret_DeleteNonExistent(t *testing.T) {
	setupEnv(t)
	// Use a unique name that definitely doesn't exist
	secretName := "suve-e2e-delete/definitely-non-existent-secret-xyz"

	// Try to delete (should fail since it doesn't exist)
	// Note: AWS Secrets Manager returns ResourceNotFoundException for non-existent secrets
	_, _, err := runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	// Localstack may return success for non-existent secrets, so we just log the result
	if err != nil {
		t.Logf("Delete non-existent returned error as expected: %v", err)
	}
}

// TestSecret_DeleteWithRecoveryWindow tests secret delete with recovery window.
func TestSecret_DeleteWithRecoveryWindow(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-delete/scheduled"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create secret
	_, _, err := runCommand(t, secretcreate.Command(), secretName, "value")
	require.NoError(t, err)

	// Delete with recovery window (7 days minimum)
	stdout, _, err := runCommand(t, secretdelete.Command(), "--yes", "--recovery-window", "7", secretName)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Scheduled")

	// Restore it (to clean up properly)
	_, _, _ = runCommand(t, secretrestore.Command(), "--yes", secretName)
}

// TestSecret_ListJSON tests secret list with JSON output.
func TestSecret_ListJSON(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-list/json-test"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	// Create secret
	_, _, _ = runCommand(t, secretcreate.Command(), secretName, "v1")

	// List with JSON format
	stdout, _, err := runCommand(t, secretlist.Command(), "--output", "json")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(stdout), "[") || strings.HasPrefix(strings.TrimSpace(stdout), "{"))
}

// TestSecret_StashPushAndPop tests service-specific stash push and pop for secrets.
func TestSecret_StashPushAndPop(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-secret-stash-push-pop/test"

	// Cleanup
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
		_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	})

	// Stage a secret
	t.Run("stage-secret", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "add", secretName, "secret-value")
		require.NoError(t, err)
		t.Logf("stage add output: %s", stdout)
	})

	// Stash push only secret service to file
	t.Run("stash-push-secret-only", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "stash", "push")
		require.NoError(t, err)
		t.Logf("stash push output: %s", stdout)
		assert.Contains(t, stdout, "stashed to file")
	})

	// Agent should be empty for secret
	t.Run("verify-agent-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, secretName)
	})

	// Stash pop secret service back from file
	t.Run("stash-pop-secret-only", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "stash", "pop")
		require.NoError(t, err)
		t.Logf("stash pop output: %s", stdout)
		assert.Contains(t, stdout, "restored")
	})

	// Secret should be back in agent
	t.Run("verify-secret-restored", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, secretName)
	})
}
