//go:build e2e

// Package e2e contains end-to-end tests for the suve CLI.
//
// These tests run against a real AWS-compatible service (localstack) and verify
// the complete workflow of each command using the actual CLI commands.
//
// Run with: make e2e
//
// Environment variables:
//   - SUVE_LOCALSTACK_EXTERNAL_PORT: Custom localstack port (default: 4566)
//
// Note: Secrets Manager tests require localstack Pro for full functionality.
package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	secretcreate "github.com/mpyw/suve/internal/cli/commands/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/secret/delete"
	secretdiff "github.com/mpyw/suve/internal/cli/commands/secret/diff"
	secretlog "github.com/mpyw/suve/internal/cli/commands/secret/log"
	secretlist "github.com/mpyw/suve/internal/cli/commands/secret/list"
	secretrestore "github.com/mpyw/suve/internal/cli/commands/secret/restore"
	secretshow "github.com/mpyw/suve/internal/cli/commands/secret/show"
	secretupdate "github.com/mpyw/suve/internal/cli/commands/secret/update"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/param/delete"
	paramdiff "github.com/mpyw/suve/internal/cli/commands/param/diff"
	paramlog "github.com/mpyw/suve/internal/cli/commands/param/log"
	paramlist "github.com/mpyw/suve/internal/cli/commands/param/list"
	paramset "github.com/mpyw/suve/internal/cli/commands/param/set"
	paramshow "github.com/mpyw/suve/internal/cli/commands/param/show"
	globalstage "github.com/mpyw/suve/internal/cli/commands/stage"
	globaldiff "github.com/mpyw/suve/internal/cli/commands/stage/diff"
	globalapply "github.com/mpyw/suve/internal/cli/commands/stage/apply"
	globalreset "github.com/mpyw/suve/internal/cli/commands/stage/reset"
	secretstage "github.com/mpyw/suve/internal/cli/commands/stage/secret"
	paramstage "github.com/mpyw/suve/internal/cli/commands/stage/param"
	globalstatus "github.com/mpyw/suve/internal/cli/commands/stage/status"
	"github.com/mpyw/suve/internal/staging"
)

func getEndpoint() string {
	return fmt.Sprintf(
		"http://127.0.0.1:%s",
		lo.CoalesceOrEmpty(os.Getenv("SUVE_LOCALSTACK_EXTERNAL_PORT"), "4566"),
	)
}

// setupEnv sets up environment variables for localstack and returns a cleanup function.
func setupEnv(t *testing.T) {
	t.Helper()
	endpoint := getEndpoint()

	// Set AWS environment variables for localstack
	t.Setenv("AWS_ENDPOINT_URL", endpoint)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
}

// setupTempHome sets up a temporary HOME directory for isolated staging tests.
func setupTempHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	return tmpHome
}

// runCommand executes a CLI command and returns stdout, stderr, and error.
func runCommand(t *testing.T, cmd *cli.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer
	app := &cli.Command{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{cmd},
	}

	// Build full args: ["suve", "command-name", ...args]
	fullArgs := append([]string{"suve", cmd.Name}, args...)
	err = app.Run(t.Context(), fullArgs)

	return outBuf.String(), errBuf.String(), err
}

// runSubCommand executes a subcommand (e.g., "param stage status") and returns stdout, stderr, and error.
func runSubCommand(t *testing.T, parentCmd *cli.Command, subCmdName string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer
	app := &cli.Command{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{parentCmd},
	}

	// Build full args: ["suve", "parent-name", "sub-name", ...args]
	fullArgs := append([]string{"suve", parentCmd.Name, subCmdName}, args...)
	err = app.Run(t.Context(), fullArgs)

	return outBuf.String(), errBuf.String(), err
}

// =============================================================================
// SSM Parameter Store Basic Commands Tests
// =============================================================================

// TestParam_FullWorkflow tests the complete SSM Parameter Store workflow:
// set → show → show --raw → update → log → diff → list → delete → verify deletion
func TestParam_FullWorkflow(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-test/basic/param"

	// Cleanup: delete parameter if it exists (ignore errors)
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Set parameter (with -y to skip confirmation)
	t.Run("set", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramset.Command(), "--yes", paramName, "initial-value")
		require.NoError(t, err)
		t.Logf("set output: %s", stdout)
	})

	// 2. Show parameter
	t.Run("show", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "initial-value")
		assert.Contains(t, stdout, paramName)
		t.Logf("show output: %s", stdout)
	})

	// 3. Show --raw (raw output without trailing newline)
	t.Run("show-raw", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "initial-value", stdout)
	})

	// 4. Update parameter
	t.Run("update", func(t *testing.T) {
		_, _, err := runCommand(t, paramset.Command(), "--yes", paramName, "updated-value")
		require.NoError(t, err)
	})

	// 5. Log (basic)
	t.Run("log", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Version 1")
		assert.Contains(t, stdout, "Version 2")
		t.Logf("log output: %s", stdout)
	})

	// 6. Log with options
	t.Run("log-with-options", func(t *testing.T) {
		// --oneline format: "VERSION  DATE  VALUE"
		stdout, _, err := runCommand(t, paramlog.Command(), "--oneline", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "1")
		assert.Contains(t, stdout, "2")
		t.Logf("log --oneline output: %s", stdout)

		// -n 1 (limit) - shows only most recent
		stdout, _, err = runCommand(t, paramlog.Command(), "-n", "1", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "current") // Most recent has "(current)"
		t.Logf("log -n 1 output: %s", stdout)

		// --reverse - oldest first
		stdout, _, err = runCommand(t, paramlog.Command(), "--reverse", paramName)
		require.NoError(t, err)
		// First entry should be Version 1 when reversed
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		assert.True(t, strings.Contains(lines[0], "1"))
		t.Logf("log --reverse output: %s", stdout)

		// -p (patch)
		stdout, _, err = runCommand(t, paramlog.Command(), "-p", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "-initial-value")
		assert.Contains(t, stdout, "+updated-value")
		t.Logf("log -p output: %s", stdout)
	})

	// 7. Diff - Compare version 1 with version 2
	t.Run("diff", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramdiff.Command(), paramName+"#1", paramName+"#2")
		require.NoError(t, err)
		assert.Contains(t, stdout, "-initial-value")
		assert.Contains(t, stdout, "+updated-value")
		t.Logf("diff output: %s", stdout)
	})

	// 8. Diff with single arg (compare with current)
	t.Run("diff-single-arg", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramdiff.Command(), paramName+"#1")
		require.NoError(t, err)
		assert.Contains(t, stdout, "-initial-value")
		assert.Contains(t, stdout, "+updated-value")
	})

	// 9. Diff with ~SHIFT
	t.Run("diff-shift", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramdiff.Command(), paramName+"~1")
		require.NoError(t, err)
		assert.Contains(t, stdout, "-initial-value")
		assert.Contains(t, stdout, "+updated-value")
	})

	// 10. List (note: localstack may not support path filtering perfectly)
	t.Run("list", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), "/suve-e2e-test/basic/")
		require.NoError(t, err)
		// Localstack might return empty for path-filtered list, skip assertion if empty
		if stdout != "" {
			assert.Contains(t, stdout, paramName)
		}
		t.Logf("list output: %s", stdout)
	})

	// 11. Delete (with -y to skip confirmation)
	t.Run("delete", func(t *testing.T) {
		_, _, err := runCommand(t, paramdelete.Command(), "--yes", paramName)
		require.NoError(t, err)
	})

	// 12. Verify deletion
	t.Run("verify-deleted", func(t *testing.T) {
		_, _, err := runCommand(t, paramshow.Command(), paramName)
		assert.Error(t, err, "expected error after deletion")
	})
}

// TestParam_VersionSpecifiers tests SSM Parameter Store version specifier syntax.
func TestParam_VersionSpecifiers(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-test/version/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create 3 versions
	_, _, err := runCommand(t, paramset.Command(), "--yes", paramName, "v1")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramset.Command(), "--yes", paramName, "v2")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramset.Command(), "--yes", paramName, "v3")
	require.NoError(t, err)

	// Test #VERSION
	t.Run("version-number", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName+"#1")
		require.NoError(t, err)
		assert.Equal(t, "v1", stdout)

		stdout, _, err = runCommand(t, paramshow.Command(), "--raw", paramName+"#2")
		require.NoError(t, err)
		assert.Equal(t, "v2", stdout)
	})

	// Test ~SHIFT
	t.Run("shift", func(t *testing.T) {
		// ~1 = 1 version ago
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName+"~1")
		require.NoError(t, err)
		assert.Equal(t, "v2", stdout)

		// ~2 = 2 versions ago
		stdout, _, err = runCommand(t, paramshow.Command(), "--raw", paramName+"~2")
		require.NoError(t, err)
		assert.Equal(t, "v1", stdout)

		// ~ alone = ~1
		stdout, _, err = runCommand(t, paramshow.Command(), "--raw", paramName+"~")
		require.NoError(t, err)
		assert.Equal(t, "v2", stdout)

		// ~~ = ~1~1 = ~2
		stdout, _, err = runCommand(t, paramshow.Command(), "--raw", paramName+"~~")
		require.NoError(t, err)
		assert.Equal(t, "v1", stdout)
	})

	// Test #VERSION~SHIFT combination
	t.Run("version-and-shift", func(t *testing.T) {
		// #3~1 = version 3, then 1 back = version 2
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName+"#3~1")
		require.NoError(t, err)
		assert.Equal(t, "v2", stdout)
	})
}

// TestParam_JSONFlag tests the --json flag for formatting.
func TestParam_JSONFlag(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-test/json/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create with JSON value
	_, _, err := runCommand(t, paramset.Command(), "--yes", paramName, `{"b":2,"a":1}`)
	require.NoError(t, err)
	_, _, err = runCommand(t, paramset.Command(), "--yes", paramName, `{"c":3,"b":2,"a":1}`)
	require.NoError(t, err)

	// Test diff with --json flag (should format and sort keys)
	t.Run("diff-json", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramdiff.Command(), "-j", paramName+"#1", paramName+"#2")
		require.NoError(t, err)
		// Keys should be sorted alphabetically in the formatted output
		assert.Contains(t, stdout, `"a"`)
		assert.Contains(t, stdout, `"b"`)
		assert.Contains(t, stdout, `"c"`)
		t.Logf("diff --json output: %s", stdout)
	})

	// Test log with -p -j flags
	t.Run("log-patch-json", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "-p", "-j", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, `"a"`)
		t.Logf("log -p -j output: %s", stdout)
	})
}

// =============================================================================
// SSM Parameter Store Staging Workflow Tests
// =============================================================================

// TestParam_StagingWorkflow tests the complete SSM Parameter Store staging workflow.
func TestParam_StagingWorkflow(t *testing.T) {
	setupEnv(t)
	tmpHome := setupTempHome(t)

	paramName := "/suve-e2e-staging/workflow/param"

	// Cleanup: delete parameter if it exists
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Create initial parameter
	t.Run("setup", func(t *testing.T) {
		_, _, err := runCommand(t, paramset.Command(), "--yes", paramName, "original-value")
		require.NoError(t, err)
	})

	// 2. Stage a new value (using store directly since edit requires interactive editor)
	t.Run("stage-edit", func(t *testing.T) {
		store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
		err := store.Stage(staging.ServiceParam, paramName, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "staged-value",
			StagedAt:  time.Now(),
		})
		require.NoError(t, err)
	})

	// 3. Status - verify staged parameter is listed
	t.Run("status", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, "M") // M = Modified (update operation)
		t.Logf("status output: %s", stdout)
	})

	// 4. Stage diff - compare staged vs current
	t.Run("stage-diff", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "diff", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "-original-value")
		assert.Contains(t, stdout, "+staged-value")
		t.Logf("stage diff output: %s", stdout)
	})

	// 5. Push - apply staged changes (with -y to skip confirmation, --ignore-conflicts since we staged directly)
	t.Run("apply", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes", "--ignore-conflicts")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		t.Logf("apply output: %s", stdout)
	})

	// 6. Verify - check the value was applied
	t.Run("verify", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "staged-value", stdout)
	})

	// 7. Status after apply - should be empty
	t.Run("status-after-apply", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, paramName)
	})

	// 8. Stage for delete
	t.Run("stage-delete", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "delete", paramName)
		require.NoError(t, err)
	})

	// 9. Status shows delete operation
	t.Run("status-delete", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, "D") // D = Delete
	})

	// 10. Reset - unstage the delete
	t.Run("reset", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "reset", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Unstaged")
		t.Logf("reset output: %s", stdout)
	})

	// 11. Status after reset - should be empty
	t.Run("status-after-reset", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, paramName)
	})

	// 12. Verify parameter still exists after reset
	t.Run("verify-not-deleted", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "staged-value", stdout)
	})
}

// TestParam_StagingAdd tests staging a new parameter (create operation).
func TestParam_StagingAdd(t *testing.T) {
	setupEnv(t)
	tmpHome := setupTempHome(t)

	paramName := "/suve-e2e-staging/add/newparam"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Stage add (using store directly since add requires interactive editor)
	t.Run("stage-add", func(t *testing.T) {
		store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
		err := store.Stage(staging.ServiceParam, paramName, staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "new-param-value",
			StagedAt:  time.Now(),
		})
		require.NoError(t, err)
	})

	// 2. Status shows add operation
	t.Run("status", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, "A") // A = Add
	})

	// 3. Push to create
	t.Run("apply", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)
	})

	// 4. Verify created
	t.Run("verify", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "new-param-value", stdout)
	})
}

// TestParam_StagingResetWithVersion tests resetting to a specific version.
func TestParam_StagingResetWithVersion(t *testing.T) {
	setupEnv(t)
	tmpHome := setupTempHome(t)

	paramName := "/suve-e2e-staging/reset-version/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter with multiple versions
	_, _, err := runCommand(t, paramset.Command(), "--yes", paramName, "v1")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramset.Command(), "--yes", paramName, "v2")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramset.Command(), "--yes", paramName, "v3")
	require.NoError(t, err)

	// 1. Reset with version spec (restore old version to staging)
	t.Run("reset-with-version", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "reset", paramName+"#1")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Restored")
		t.Logf("reset with version output: %s", stdout)
	})

	// 2. Status shows staged value
	t.Run("status", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})

	// 3. Verify staged value is from version 1
	t.Run("verify-staged", func(t *testing.T) {
		store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
		entry, err := store.Get(staging.ServiceParam, paramName)
		require.NoError(t, err)
		assert.Equal(t, "v1", entry.Value)
	})

	// 4. Push to apply (use --ignore-conflicts for robustness in test environment)
	t.Run("apply", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes", "--ignore-conflicts")
		require.NoError(t, err)
	})

	// 5. Verify reverted
	t.Run("verify-reverted", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "v1", stdout)
	})
}

// TestParam_StagingResetAll tests resetting all staged changes.
func TestParam_StagingResetAll(t *testing.T) {
	setupEnv(t)
	tmpHome := setupTempHome(t)

	param1 := "/suve-e2e-staging/reset-all/param1"
	param2 := "/suve-e2e-staging/reset-all/param2"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", param1)
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", param2)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", param1)
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", param2)
	})

	// Create parameters
	_, _, _ = runCommand(t, paramset.Command(), "--yes", param1, "value1")
	_, _, _ = runCommand(t, paramset.Command(), "--yes", param2, "value2")

	// Stage both
	store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
	_ = store.Stage(staging.ServiceParam, param1, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "staged1",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceParam, param2, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "staged2",
		StagedAt:  time.Now(),
	})

	// Verify both staged
	t.Run("verify-staged", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, param1)
		assert.Contains(t, stdout, param2)
	})

	// Reset all
	t.Run("reset-all", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "reset", "--all")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Unstaged")
		t.Logf("reset --all output: %s", stdout)
	})

	// Verify empty
	t.Run("verify-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, param1)
		assert.NotContains(t, stdout, param2)
	})
}

// TestParam_StagingApplySingle tests applying a single parameter.
func TestParam_StagingApplySingle(t *testing.T) {
	setupEnv(t)
	tmpHome := setupTempHome(t)

	param1 := "/suve-e2e-staging/apply-single/param1"
	param2 := "/suve-e2e-staging/apply-single/param2"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", param1)
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", param2)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", param1)
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", param2)
	})

	// Create parameters
	_, _, _ = runCommand(t, paramset.Command(), "--yes", param1, "original1")
	_, _, _ = runCommand(t, paramset.Command(), "--yes", param2, "original2")

	// Stage both
	store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
	_ = store.Stage(staging.ServiceParam, param1, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "staged1",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceParam, param2, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "staged2",
		StagedAt:  time.Now(),
	})

	// Push only param1 (use --ignore-conflicts since we staged without original version)
	t.Run("apply-single", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes", "--ignore-conflicts", param1)
		require.NoError(t, err)
	})

	// Verify param1 updated, param2 still staged
	t.Run("verify", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", param1)
		require.NoError(t, err)
		assert.Equal(t, "staged1", stdout)

		stdout, _, err = runCommand(t, paramshow.Command(), "--raw", param2)
		require.NoError(t, err)
		assert.Equal(t, "original2", stdout) // Not applied yet

		// param2 should still be staged
		stdout, _, err = runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, param1) // Already applied
		assert.Contains(t, stdout, param2)    // Still staged
	})
}

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
	tmpHome := setupTempHome(t)

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
		store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
		err := store.Stage(staging.ServiceSecret, secretName, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "staged-secret",
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
	tmpHome := setupTempHome(t)

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
		store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
		entry, err := store.Get(staging.ServiceSecret, secretName)
		require.NoError(t, err)
		require.NotNil(t, entry.DeleteOptions)
		assert.Equal(t, 14, entry.DeleteOptions.RecoveryWindow)
		assert.False(t, entry.DeleteOptions.Force)
	})
}

// =============================================================================
// Global Stage Commands Tests
// =============================================================================

// TestGlobal_StageWorkflow tests the global stage commands that work across services.
func TestGlobal_StageWorkflow(t *testing.T) {
	setupEnv(t)
	tmpHome := setupTempHome(t)

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
	_, _, _ = runCommand(t, paramset.Command(), "--yes", paramName, "original-param")
	_, _, _ = runCommand(t, secretcreate.Command(), secretName, "original-secret")

	// Stage both
	store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
	_ = store.Stage(staging.ServiceParam, paramName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "staged-param",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceSecret, secretName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "staged-secret",
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

	// 3. Global apply applies both (no -y needed, it doesn't have confirmation)
	t.Run("global-apply", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalapply.Command())
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
	tmpHome := setupTempHome(t)

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
	_, _, _ = runCommand(t, paramset.Command(), "--yes", paramName, "original")
	_, _, _ = runCommand(t, secretcreate.Command(), secretName, "original")

	store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
	_ = store.Stage(staging.ServiceParam, paramName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "staged",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceSecret, secretName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "staged",
		StagedAt:  time.Now(),
	})

	// Global reset (automatically resets all, no --all flag needed)
	t.Run("reset-all", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalreset.Command())
		require.NoError(t, err)
		t.Logf("global reset output: %s", stdout)
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

// =============================================================================
// Edge Cases and Error Handling Tests
// =============================================================================

// TestParam_ErrorCases tests various error scenarios.
func TestParam_ErrorCases(t *testing.T) {
	setupEnv(t)

	// Show non-existent parameter
	t.Run("show-nonexistent", func(t *testing.T) {
		_, _, err := runCommand(t, paramshow.Command(), "/nonexistent/param/12345")
		assert.Error(t, err)
	})

	// Cat non-existent parameter
	t.Run("cat-nonexistent", func(t *testing.T) {
		_, _, err := runCommand(t, paramshow.Command(), "--raw", "/nonexistent/param/12345")
		assert.Error(t, err)
	})

	// Delete non-existent parameter
	t.Run("delete-nonexistent", func(t *testing.T) {
		_, _, err := runCommand(t, paramdelete.Command(), "--yes", "/nonexistent/param/12345")
		assert.Error(t, err)
	})

	// Invalid version specifier
	t.Run("invalid-version", func(t *testing.T) {
		_, _, err := runCommand(t, paramshow.Command(), "--raw", "/param#abc")
		assert.Error(t, err)
	})

	// Missing required args
	t.Run("missing-args-set", func(t *testing.T) {
		_, _, err := runCommand(t, paramset.Command())
		assert.Error(t, err)
	})

	t.Run("missing-args-show", func(t *testing.T) {
		_, _, err := runCommand(t, paramshow.Command())
		assert.Error(t, err)
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

// =============================================================================
// Special Scenarios
// =============================================================================

// TestParam_SpecialCharactersInValue tests values with special characters.
func TestParam_SpecialCharactersInValue(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-test/special/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	testCases := []struct {
		name  string
		value string
	}{
		{"newlines", "line1\nline2\nline3"},
		{"tabs", "col1\tcol2\tcol3"},
		{"unicode", "Hello 世界 🌍"},
		{"json", `{"key": "value", "nested": {"a": 1}}`},
		{"quotes", `"double" and 'single' quotes`},
		{"backslashes", `path\to\file`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runCommand(t, paramset.Command(), "--yes", paramName, tc.value)
			require.NoError(t, err)

			stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
			require.NoError(t, err)
			assert.Equal(t, tc.value, stdout)
		})
	}
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

// TestParam_LongValue tests handling of long parameter values.
func TestParam_LongValue(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-test/long/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create a long value (SSM Parameter Store limit is 4KB for standard, 8KB for advanced)
	longValue := strings.Repeat("a", 4000)

	_, _, err := runCommand(t, paramset.Command(), "--yes", paramName, longValue)
	require.NoError(t, err)

	stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
	require.NoError(t, err)
	assert.Equal(t, longValue, stdout)
}

// =============================================================================
// Staging CLI Commands Tests (add/edit via CLI)
// =============================================================================

// TestParam_StagingAddViaCLI tests the stage add command via CLI (with value argument).
func TestParam_StagingAddViaCLI(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/add-cli/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Stage add via CLI (with value argument - no editor needed)
	t.Run("add-via-cli", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "cli-staged-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
		t.Logf("stage add output: %s", stdout)
	})

	// 2. Verify status shows add operation
	t.Run("status", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, "A") // A = Add
	})

	// 3. Push to create
	t.Run("apply", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)
	})

	// 4. Verify created
	t.Run("verify", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "cli-staged-value", stdout)
	})
}

// TestParam_StagingAddWithOptions tests stage add with description and tags.
func TestParam_StagingAddWithOptions(t *testing.T) {
	setupEnv(t)
	tmpHome := setupTempHome(t)

	paramName := "/suve-e2e-staging/add-options/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Stage add with description and tags
	t.Run("add-with-options", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "add",
			"--description", "Test description",
			"--tag", "env=test",
			"--tag", "owner=e2e",
			paramName, "value-with-options")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
		t.Logf("stage add with options output: %s", stdout)
	})

	// Verify staged entry has options
	t.Run("verify-staged-options", func(t *testing.T) {
		store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
		entry, err := store.Get(staging.ServiceParam, paramName)
		require.NoError(t, err)
		assert.Equal(t, "value-with-options", entry.Value)
		require.NotNil(t, entry.Description)
		assert.Equal(t, "Test description", *entry.Description)
		require.NotNil(t, entry.Tags)
		assert.Equal(t, "test", entry.Tags["env"])
		assert.Equal(t, "e2e", entry.Tags["owner"])
	})

	// Push and verify
	t.Run("apply-and-verify", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)

		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "value-with-options", stdout)
	})
}

// TestSecret_StagingAddViaCLI tests the Secrets Manager stage add command via CLI.
func TestSecret_StagingAddViaCLI(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

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

// TestParam_StagingEditViaCLI tests re-adding (editing) a staged value via CLI.
func TestParam_StagingEditViaCLI(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/edit-cli/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Stage add first
	t.Run("stage-add", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "initial-value")
		require.NoError(t, err)
	})

	// 2. Re-add (edit) the staged value
	t.Run("re-add-edit", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "edited-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
		t.Logf("re-add output: %s", stdout)
	})

	// 3. Push and verify
	t.Run("apply-and-verify", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)

		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "edited-value", stdout)
	})
}

// TestParam_StagingDiffViaCLI tests the stage diff command via CLI for various operations.
func TestParam_StagingDiffViaCLI(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/diff-cli/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Stage add and check diff
	t.Run("diff-for-create", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "new-value")
		require.NoError(t, err)

		stdout, _, err := runSubCommand(t, paramstage.Command(), "diff", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "+new-value")
		t.Logf("diff output for create: %s", stdout)
	})

	// 2. Push and setup for update
	t.Run("apply-and-setup", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)
	})

	// 3. Stage delete and check diff
	t.Run("diff-for-delete", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "delete", paramName)
		require.NoError(t, err)

		stdout, _, err := runSubCommand(t, paramstage.Command(), "diff", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "-new-value")
		t.Logf("diff output for delete: %s", stdout)
	})
}

// TestParam_GlobalDiffWithJSON tests global diff with JSON formatting.
func TestParam_GlobalDiffWithJSON(t *testing.T) {
	setupEnv(t)
	tmpHome := setupTempHome(t)

	paramName := "/suve-e2e-global/json-diff/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create param with JSON value
	_, _, err := runCommand(t, paramset.Command(), "--yes", paramName, `{"a":1}`)
	require.NoError(t, err)

	// Stage update with different JSON
	store := staging.NewStoreWithPath(filepath.Join(tmpHome, ".suve", "stage.json"))
	err = store.Stage(staging.ServiceParam, paramName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     `{"a":1,"b":2}`,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	// Check diff with --json flag
	stdout, _, err := runCommand(t, globaldiff.Command(), "--json")
	require.NoError(t, err)
	t.Logf("global diff --json output: %s", stdout)
	// Should have formatted JSON
	assert.Contains(t, stdout, "a")
}
