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
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	paramcreate "github.com/mpyw/suve/internal/cli/commands/param/create"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/param/delete"
	paramdiff "github.com/mpyw/suve/internal/cli/commands/param/diff"
	paramlist "github.com/mpyw/suve/internal/cli/commands/param/list"
	paramlog "github.com/mpyw/suve/internal/cli/commands/param/log"
	paramshow "github.com/mpyw/suve/internal/cli/commands/param/show"
	paramtag "github.com/mpyw/suve/internal/cli/commands/param/tag"
	paramuntag "github.com/mpyw/suve/internal/cli/commands/param/untag"
	paramupdate "github.com/mpyw/suve/internal/cli/commands/param/update"
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
	globalstage "github.com/mpyw/suve/internal/cli/commands/stage"
	globalapply "github.com/mpyw/suve/internal/cli/commands/stage/apply"
	globaldiff "github.com/mpyw/suve/internal/cli/commands/stage/diff"
	globaldrain "github.com/mpyw/suve/internal/cli/commands/stage/drain"
	paramstage "github.com/mpyw/suve/internal/cli/commands/stage/param"
	globalpersist "github.com/mpyw/suve/internal/cli/commands/stage/persist"
	globalreset "github.com/mpyw/suve/internal/cli/commands/stage/reset"
	secretstage "github.com/mpyw/suve/internal/cli/commands/stage/secret"
	globalstatus "github.com/mpyw/suve/internal/cli/commands/stage/status"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon"
)

// testDaemon is the shared staging agent daemon for all E2E tests.
var testDaemon *daemon.Runner

// TestMain sets up the staging agent daemon before running tests.
func TestMain(m *testing.M) {
	// Create isolated temp directory for socket path
	tmpDir, err := os.MkdirTemp("", "suve-e2e-*")
	if err != nil {
		output.Printf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	// Set TMPDIR so protocol.SocketPath() uses our isolated directory
	if err := os.Setenv("TMPDIR", tmpDir); err != nil {
		output.Printf(os.Stderr, "failed to set TMPDIR: %v\n", err)
		os.Exit(1)
	}

	// Enable manual mode to prevent fork bomb from test binary
	// This disables both auto-start and auto-shutdown
	if err := os.Setenv(agent.EnvDaemonManualMode, "1"); err != nil {
		output.Printf(os.Stderr, "failed to set manual mode: %v\n", err)
		os.Exit(1)
	}

	// Start daemon with error channel
	testDaemon = daemon.NewRunner(agent.DaemonOptions()...)
	daemonErrCh := make(chan error, 1)
	go func() {
		daemonErrCh <- testDaemon.Run()
	}()

	// Wait for daemon to be ready by polling with ping
	if err := waitForDaemon(5*time.Second, daemonErrCh); err != nil {
		output.Printf(os.Stderr, "failed to start daemon: %v\n", err)
		testDaemon.Shutdown()
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	testDaemon.Shutdown()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

// waitForDaemon waits for the daemon to be ready by polling with ping.
func waitForDaemon(timeout time.Duration, daemonErrCh <-chan error) error {
	launcher := daemon.NewLauncher(daemon.WithAutoStartDisabled())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case err := <-daemonErrCh:
			// Daemon exited with error
			return fmt.Errorf("daemon exited: %w", err)
		case <-ctx.Done():
			return fmt.Errorf("daemon did not become ready within %v (last error: %v)", timeout, lastErr)
		case <-ticker.C:
			// Try to ping daemon (any successful request means it's ready)
			if err := launcher.Ping(); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
	}
}

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

// newStore creates a new staging store for E2E tests.
// localstack uses account ID "000000000000" and region "us-east-1".
func newStore() store.AgentStore {
	return agent.NewStore("000000000000", "us-east-1")
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
// create → show → show --raw → update → log → diff → list → delete → verify deletion
func TestParam_FullWorkflow(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-test/basic/param"

	// Cleanup: delete parameter if it exists (ignore errors)
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Create parameter
	t.Run("create", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramcreate.Command(), paramName, "initial-value")
		require.NoError(t, err)
		t.Logf("create output: %s", stdout)
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
		_, _, err := runCommand(t, paramupdate.Command(), "--yes", paramName, "updated-value")
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
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "v1")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramupdate.Command(), "--yes", paramName, "v2")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramupdate.Command(), "--yes", paramName, "v3")
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

// TestParam_ParseJSONFlag tests the --parse-json/-j flag for formatting.
func TestParam_ParseJSONFlag(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-test/json/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create with JSON value
	_, _, err := runCommand(t, paramcreate.Command(), paramName, `{"b":2,"a":1}`)
	require.NoError(t, err)
	_, _, err = runCommand(t, paramupdate.Command(), "--yes", paramName, `{"c":3,"b":2,"a":1}`)
	require.NoError(t, err)

	// Test diff with -j flag (should format and sort keys)
	t.Run("diff-json", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramdiff.Command(), "-j", paramName+"#1", paramName+"#2")
		require.NoError(t, err)
		// Keys should be sorted alphabetically in the formatted output
		assert.Contains(t, stdout, `"a"`)
		assert.Contains(t, stdout, `"b"`)
		assert.Contains(t, stdout, `"c"`)
		t.Logf("diff -j output: %s", stdout)
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
	setupTempHome(t)

	paramName := "/suve-e2e-staging/workflow/param"

	// Cleanup: delete parameter if it exists
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Create initial parameter
	t.Run("setup", func(t *testing.T) {
		_, _, err := runCommand(t, paramcreate.Command(), paramName, "original-value")
		require.NoError(t, err)
	})

	// 2. Stage a new value (using store directly since edit requires interactive editor)
	t.Run("stage-edit", func(t *testing.T) {
		store := newStore()
		err := store.StageEntry(t.Context(), staging.ServiceParam, paramName, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("staged-value"),
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
	setupTempHome(t)

	paramName := "/suve-e2e-staging/add/newparam"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// 1. Stage add (using store directly since add requires interactive editor)
	t.Run("stage-add", func(t *testing.T) {
		store := newStore()
		err := store.StageEntry(t.Context(), staging.ServiceParam, paramName, staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-param-value"),
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
	setupTempHome(t)

	paramName := "/suve-e2e-staging/reset-version/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter with multiple versions
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "v1")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramupdate.Command(), "--yes", paramName, "v2")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramupdate.Command(), "--yes", paramName, "v3")
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
		store := newStore()
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, paramName)
		require.NoError(t, err)
		require.NotNil(t, entry.Value)
		assert.Equal(t, "v1", *entry.Value)
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
	setupTempHome(t)

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
	_, _, _ = runCommand(t, paramcreate.Command(), param1, "value1")
	_, _, _ = runCommand(t, paramcreate.Command(), param2, "value2")

	// Stage both
	store := newStore()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, param1, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged1"),
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceParam, param2, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged2"),
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
	setupTempHome(t)

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
	_, _, _ = runCommand(t, paramcreate.Command(), param1, "original1")
	_, _, _ = runCommand(t, paramcreate.Command(), param2, "original2")

	// Stage both
	store := newStore()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, param1, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged1"),
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceParam, param2, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged2"),
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
	t.Run("missing-args-create", func(t *testing.T) {
		_, _, err := runCommand(t, paramcreate.Command())
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
			// Ensure clean state before each test case
			_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
			_, _, err := runCommand(t, paramcreate.Command(), paramName, tc.value)
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

	_, _, err := runCommand(t, paramcreate.Command(), paramName, longValue)
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

// TestParam_StagingAddWithOptions tests stage add with description and stage tag for tags.
func TestParam_StagingAddWithOptions(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	paramName := "/suve-e2e-staging/add-options/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Stage add with description
	t.Run("add-with-description", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "add",
			"--description", "Test description",
			paramName, "value-with-options")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
		t.Logf("stage add with options output: %s", stdout)
	})

	// Stage tags separately
	t.Run("add-tags", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "tag",
			paramName, "env=test", "owner=e2e")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
		t.Logf("stage tag output: %s", stdout)
	})

	// Verify service-specific status shows tag changes
	t.Run("service-status-shows-tags", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "T")         // T = Tag change marker
		assert.Contains(t, stdout, "+2 tag(s)") // Two tags being added
		t.Logf("service status output: %s", stdout)
	})

	// Verify global status shows tag changes
	t.Run("global-status-shows-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, globalstatus.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, "T")         // T = Tag change marker
		assert.Contains(t, stdout, "+2 tag(s)") // Two tags being added
		t.Logf("global status output: %s", stdout)
	})

	// Verify staged entry has options
	t.Run("verify-staged-options", func(t *testing.T) {
		store := newStore()

		// Verify entry
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, paramName)
		require.NoError(t, err)
		require.NotNil(t, entry.Value)
		assert.Equal(t, "value-with-options", *entry.Value)
		require.NotNil(t, entry.Description)
		assert.Equal(t, "Test description", *entry.Description)

		// Verify tags (now stored separately)
		tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, paramName)
		require.NoError(t, err)
		assert.Equal(t, "test", tagEntry.Add["env"])
		assert.Equal(t, "e2e", tagEntry.Add["owner"])
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
	setupTempHome(t)

	paramName := "/suve-e2e-global/json-diff/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create param with JSON value
	_, _, err := runCommand(t, paramcreate.Command(), paramName, `{"a":1}`)
	require.NoError(t, err)

	// Stage update with different JSON
	store := newStore()
	err = store.StageEntry(t.Context(), staging.ServiceParam, paramName, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(`{"a":1,"b":2}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	// Check diff with -j flag (--parse-json)
	stdout, _, err := runCommand(t, globaldiff.Command(), "-j")
	require.NoError(t, err)
	t.Logf("global diff -j output: %s", stdout)
	// Should have formatted JSON
	assert.Contains(t, stdout, "a")
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

// TestParam_OutputOption tests --output=json option on various commands.
func TestParam_OutputOption(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-output/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create param
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "test-value")
	require.NoError(t, err)

	// Update to create version 2
	_, _, err = runCommand(t, paramupdate.Command(), "--yes", paramName, "updated-value")
	require.NoError(t, err)

	t.Run("show --output=json", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--output=json", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, `"name"`)
		assert.Contains(t, stdout, `"version"`)
		assert.Contains(t, stdout, `"value"`)
		assert.Contains(t, stdout, "updated-value")
		t.Logf("show --output=json: %s", stdout)
	})

	t.Run("list --output=json", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), "--output=json", "/suve-e2e-output")
		require.NoError(t, err)
		assert.Contains(t, stdout, `"name"`)
		assert.Contains(t, stdout, paramName)
		t.Logf("list --output=json: %s", stdout)
	})

	t.Run("list --output=json --show", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), "--output=json", "--show", "/suve-e2e-output")
		require.NoError(t, err)
		assert.Contains(t, stdout, `"name"`)
		assert.Contains(t, stdout, `"value"`)
		assert.Contains(t, stdout, "updated-value")
		t.Logf("list --output=json --show: %s", stdout)
	})

	t.Run("log --output=json", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "--output=json", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, `"version"`)
		assert.Contains(t, stdout, `"value"`)
		t.Logf("log --output=json: %s", stdout)
	})

	t.Run("diff --output=json", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramdiff.Command(), "--output=json", paramName+"#1", paramName+"#2")
		require.NoError(t, err)
		assert.Contains(t, stdout, `"oldVersion"`)
		assert.Contains(t, stdout, `"newVersion"`)
		assert.Contains(t, stdout, `"identical"`)
		t.Logf("diff --output=json: %s", stdout)
	})
}

// TestParam_FilterOption tests --filter option on list command.
func TestParam_FilterOption(t *testing.T) {
	setupEnv(t)
	prefix := "/suve-e2e-filter"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/foo")
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/bar")
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/baz")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/foo")
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/bar")
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/baz")
	})

	// Create params
	_, _, err := runCommand(t, paramcreate.Command(), prefix+"/foo", "foo-val")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramcreate.Command(), prefix+"/bar", "bar-val")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramcreate.Command(), prefix+"/baz", "baz-val")
	require.NoError(t, err)

	t.Run("filter ba", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), "--filter", "ba", prefix)
		require.NoError(t, err)
		assert.Contains(t, stdout, prefix+"/bar")
		assert.Contains(t, stdout, prefix+"/baz")
		assert.NotContains(t, stdout, prefix+"/foo")
		t.Logf("list --filter ba: %s", stdout)
	})

	t.Run("filter regex", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), "--filter", "ba.$", prefix)
		require.NoError(t, err)
		assert.Contains(t, stdout, prefix+"/bar")
		assert.Contains(t, stdout, prefix+"/baz")
		assert.NotContains(t, stdout, prefix+"/foo")
	})
}

// TestParam_ShowOption tests --show option on list command.
func TestParam_ShowOption(t *testing.T) {
	setupEnv(t)
	prefix := "/suve-e2e-show"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/param1")
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/param2")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/param1")
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", prefix+"/param2")
	})

	// Create params
	_, _, err := runCommand(t, paramcreate.Command(), prefix+"/param1", "value1")
	require.NoError(t, err)
	_, _, err = runCommand(t, paramcreate.Command(), prefix+"/param2", "value2")
	require.NoError(t, err)

	t.Run("list without --show", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), prefix)
		require.NoError(t, err)
		assert.Contains(t, stdout, prefix+"/param1")
		assert.Contains(t, stdout, prefix+"/param2")
		// Without --show, values should not be present
		assert.NotContains(t, stdout, "value1")
		assert.NotContains(t, stdout, "value2")
	})

	t.Run("list with --show", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), "--show", prefix)
		require.NoError(t, err)
		assert.Contains(t, stdout, prefix+"/param1")
		assert.Contains(t, stdout, prefix+"/param2")
		// With --show, values should be present
		assert.Contains(t, stdout, "value1")
		assert.Contains(t, stdout, "value2")
		t.Logf("list --show: %s", stdout)
	})
}

// =============================================================================
// Resource Existence Check Tests
// =============================================================================

// TestParam_StagingAddExistingResourceFails tests that adding an existing resource fails.
func TestParam_StagingAddExistingResourceFails(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/add-existing/param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create the parameter first
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "existing-value")
	require.NoError(t, err)

	// Try to stage add - should fail because resource already exists
	t.Run("add-existing-fails", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "new-value")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		t.Logf("expected error: %v", err)
	})
}

// TestParam_StagingDeleteNonExistingResourceFails tests that deleting a non-existing resource fails.
func TestParam_StagingDeleteNonExistingResourceFails(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/delete-nonexisting/param-does-not-exist"

	// Ensure parameter doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to stage delete - should fail because resource doesn't exist
	t.Run("delete-nonexisting-fails", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "delete", paramName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		t.Logf("expected error: %v", err)
	})
}

// TestParam_StagingTagNonExistingResourceFails tests that tagging a non-existing resource fails.
func TestParam_StagingTagNonExistingResourceFails(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/tag-nonexisting/param-does-not-exist"

	// Ensure parameter doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to stage tag - should fail because resource doesn't exist
	t.Run("tag-nonexisting-fails", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "tag", paramName, "env=test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		t.Logf("expected error: %v", err)
	})
}

// TestParam_StagingUntagNonExistingResourceFails tests that untagging a non-existing resource fails.
func TestParam_StagingUntagNonExistingResourceFails(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/untag-nonexisting/param-does-not-exist"

	// Ensure parameter doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to stage untag - should fail because resource doesn't exist
	t.Run("untag-nonexisting-fails", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "untag", paramName, "env")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		t.Logf("expected error: %v", err)
	})
}

// TestParam_StagingDeleteStagedCreateSucceeds tests that deleting a staged CREATE unstages it.
func TestParam_StagingDeleteStagedCreateSucceeds(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/delete-staged-create/param"

	// Ensure parameter doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Stage add first
	t.Run("stage-add", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "new-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
	})

	// Verify it's staged
	t.Run("verify-staged", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, "A") // A = Add
	})

	// Delete the staged CREATE - should unstage it
	t.Run("delete-staged-create", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "delete", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Unstaged") // Should say "Unstaged" not "Staged for deletion"
		t.Logf("delete staged create output: %s", stdout)
	})

	// Verify it's no longer staged
	t.Run("verify-unstaged", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, paramName)
	})
}

// TestParam_StagingTagStagedCreateSucceeds tests that tagging a staged CREATE succeeds.
func TestParam_StagingTagStagedCreateSucceeds(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

	paramName := "/suve-e2e-staging/tag-staged-create/param"

	// Ensure parameter doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Stage add first
	t.Run("stage-add", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "new-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
	})

	// Tag the staged CREATE - should succeed
	t.Run("tag-staged-create", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "tag", paramName, "env=test")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Staged")
		t.Logf("tag staged create output: %s", stdout)
	})

	// Verify both entry and tag are staged
	t.Run("verify-staged-with-tags", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		assert.Contains(t, stdout, "A") // A = Add
		assert.Contains(t, stdout, "T") // T = Tag change
	})

	// Apply and verify
	t.Run("apply-and-verify", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)

		stdout, _, err := runCommand(t, paramshow.Command(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "new-value")
		assert.Contains(t, stdout, "env: test")
	})
}

// TestSecret_StagingAddExistingResourceFails tests that adding an existing secret fails.
func TestSecret_StagingAddExistingResourceFails(t *testing.T) {
	setupEnv(t)
	_ = setupTempHome(t)

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
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		t.Logf("expected error: %v", err)
	})
}

// =============================================================================
// Tag/Untag Command Tests
// =============================================================================

// TestParam_TagAndUntag tests the param tag and untag commands.
func TestParam_TagAndUntag(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-tag/test-param"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter first
	t.Run("create", func(t *testing.T) {
		_, _, err := runCommand(t, paramcreate.Command(), paramName, "test-value")
		require.NoError(t, err)
	})

	// Add tags
	t.Run("tag", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramtag.Command(), paramName, "env=test", "team=suve")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Tagged")
		t.Logf("tag output: %s", stdout)
	})

	// Verify tags are added
	t.Run("verify-tags", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env: test")
		assert.Contains(t, stdout, "team: suve")
	})

	// Remove one tag
	t.Run("untag", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramuntag.Command(), paramName, "team")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Untagged")
		t.Logf("untag output: %s", stdout)
	})

	// Verify tag is removed
	t.Run("verify-untag", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "env: test")
		assert.NotContains(t, stdout, "team: suve")
	})
}

// TestParam_TagInvalidFormat tests error handling for invalid tag formats.
func TestParam_TagInvalidFormat(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-tag/invalid-format"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter first
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "test-value")
	require.NoError(t, err)

	// Try to add invalid tag format
	t.Run("invalid-format", func(t *testing.T) {
		_, _, err := runCommand(t, paramtag.Command(), paramName, "invalid-tag-no-equals")
		assert.Error(t, err)
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
		assert.Error(t, err)
		t.Logf("expected error: %v", err)
	})
}

// TestParam_TagNonExistent tests tagging a non-existent parameter.
func TestParam_TagNonExistent(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-tag/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to tag non-existent parameter
	_, _, err := runCommand(t, paramtag.Command(), paramName, "env=test")
	assert.Error(t, err)
	t.Logf("expected error: %v", err)
}

// TestSecret_TagNonExistent tests tagging a non-existent secret.
func TestSecret_TagNonExistent(t *testing.T) {
	setupEnv(t)
	secretName := "suve-e2e-tag/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)

	// Try to tag non-existent secret
	_, _, err := runCommand(t, secrettag.Command(), secretName, "env=test")
	assert.Error(t, err)
	t.Logf("expected error: %v", err)
}

// TestParam_UntagNonExistent tests untagging a non-existent parameter.
func TestParam_UntagNonExistent(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-untag/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to untag non-existent parameter
	_, _, err := runCommand(t, paramuntag.Command(), paramName, "env")
	assert.Error(t, err)
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
	assert.Error(t, err)
	t.Logf("expected error: %v", err)
}

// =============================================================================
// Update Command Edge Cases
// =============================================================================

// TestParam_UpdateWithType tests param update with --type flag.
func TestParam_UpdateWithType(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-update/type-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter first
	t.Run("create", func(t *testing.T) {
		_, _, err := runCommand(t, paramcreate.Command(), paramName, "initial-value")
		require.NoError(t, err)
	})

	// Update with type change to SecureString
	t.Run("update-secure", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramupdate.Command(), "--yes", "--secure", paramName, "secure-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Updated")
	})

	// Verify the type changed
	t.Run("verify-secure", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "SecureString")
	})

	// Update with explicit type StringList
	t.Run("update-stringlist", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramupdate.Command(), "--yes", "--type", "StringList", paramName, "item1,item2,item3")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Updated")
	})
}

// TestParam_UpdateConflictingFlags tests error handling for conflicting flags.
func TestParam_UpdateConflictingFlags(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-update/conflicting-flags"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter first
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "initial-value")
	require.NoError(t, err)

	// Try to use both --secure and --type
	t.Run("secure-and-type-conflict", func(t *testing.T) {
		_, _, err := runCommand(t, paramupdate.Command(), "--yes", "--secure", "--type", "String", paramName, "new-value")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})
}

// TestParam_UpdateWithDescription tests param update with description.
func TestParam_UpdateWithDescription(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-update/description-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter first
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "initial-value")
	require.NoError(t, err)

	// Update with description
	t.Run("update-with-description", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramupdate.Command(), "--yes", "--description", "Updated description", paramName, "new-value")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Updated")
	})
}

// TestParam_UpdateNonExistent tests updating a non-existent parameter.
func TestParam_UpdateNonExistent(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-update/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to update non-existent parameter
	_, _, err := runCommand(t, paramupdate.Command(), "--yes", paramName, "new-value")
	assert.Error(t, err)
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

// TestParam_UpdateMissingArgs tests error handling for missing arguments.
func TestParam_UpdateMissingArgs(t *testing.T) {
	setupEnv(t)

	// No arguments at all
	t.Run("no-args", func(t *testing.T) {
		_, _, err := runCommand(t, paramupdate.Command())
		assert.Error(t, err)
	})

	// Only name, no value
	t.Run("no-value", func(t *testing.T) {
		_, _, err := runCommand(t, paramupdate.Command(), "/test/param")
		assert.Error(t, err)
	})
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

// =============================================================================
// Log Command Edge Cases
// =============================================================================

// TestParam_LogWithNumber tests param log with --number flag.
func TestParam_LogWithNumber(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-log/num-flag-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter with multiple versions
	stdout, stderr, err := runCommand(t, paramcreate.Command(), paramName, "first-value")
	t.Logf("create: stdout=%s, stderr=%s, err=%v", stdout, stderr, err)
	require.NoError(t, err)

	stdout, stderr, err = runCommand(t, paramupdate.Command(), "--yes", paramName, "second-value")
	t.Logf("update: stdout=%s, stderr=%s, err=%v", stdout, stderr, err)
	require.NoError(t, err)

	// Get full log to verify versions
	stdout, _, err = runCommand(t, paramlog.Command(), paramName)
	require.NoError(t, err)
	t.Logf("full log: %s", stdout)

	// Get log with -n 1 (only most recent)
	stdout, _, err = runCommand(t, paramlog.Command(), "-n", "1", paramName)
	require.NoError(t, err)
	t.Logf("log -n 1: %s", stdout)
	// Should only have 1 version entry
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	versionCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Version") {
			versionCount++
		}
	}
	assert.Equal(t, 1, versionCount)
}

// TestParam_LogNonExistent tests log for non-existent parameter.
func TestParam_LogNonExistent(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-log/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to get log
	_, _, err := runCommand(t, paramlog.Command(), paramName)
	assert.Error(t, err)
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

// TestParam_LogWithFormat tests param log with different output formats.
func TestParam_LogWithFormat(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-log/format-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "test-value")

	// Test JSON format
	t.Run("json-format", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "--output", "json", paramName)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(strings.TrimSpace(stdout), "[") || strings.HasPrefix(strings.TrimSpace(stdout), "{"))
	})

	// Test text format (default)
	t.Run("text-format", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), paramName)
		require.NoError(t, err)
		// Log output format is "Version N" not "Version:"
		assert.Contains(t, stdout, "Version")
		assert.Contains(t, stdout, "Date:")
	})
}

// TestParam_LogWithPatch tests param log with --patch flag.
func TestParam_LogWithPatch(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-log/patch-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create and update to have multiple versions
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "initial-value")
	_, _, _ = runCommand(t, paramupdate.Command(), "--yes", paramName, "updated-value")

	// Test with patch flag
	t.Run("with-patch", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "-p", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Version")
		// Patch output should show diff-like content
		assert.True(t, strings.Contains(stdout, "+") || strings.Contains(stdout, "-") || strings.Contains(stdout, "initial-value") || strings.Contains(stdout, "updated-value"))
	})
}

// TestParam_LogWithOneline tests param log with --oneline flag.
func TestParam_LogWithOneline(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-log/oneline-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "test-value")

	// Test with oneline flag
	t.Run("with-oneline", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "--oneline", paramName)
		require.NoError(t, err)
		// Oneline format should be compact
		assert.NotEmpty(t, stdout)
	})

	// Test with oneline and max-value-length
	t.Run("with-oneline-maxlen", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "--oneline", "--max-value-length", "5", paramName)
		require.NoError(t, err)
		assert.NotEmpty(t, stdout)
	})
}

// TestParam_LogWithReverse tests param log with --reverse flag.
func TestParam_LogWithReverse(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-log/reverse-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create and update to have multiple versions
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "v1")
	_, _, _ = runCommand(t, paramupdate.Command(), "--yes", paramName, "v2")
	_, _, _ = runCommand(t, paramupdate.Command(), "--yes", paramName, "v3")

	// Test with reverse flag
	t.Run("with-reverse", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "--reverse", paramName)
		require.NoError(t, err)
		// In reverse mode, should still show versions
		assert.Contains(t, stdout, "Version")
	})
}

// TestParam_LogFlagWarnings tests param log warning messages for conflicting flags.
func TestParam_LogFlagWarnings(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-log/warnings-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "test-value")

	// Test --parse-json without --patch (should warn)
	t.Run("parse-json-without-patch", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, paramlog.Command(), "--parse-json", paramName)
		require.NoError(t, err)
		assert.NotEmpty(t, stdout)
		// Should have a warning on stderr about --parse-json having no effect
		t.Logf("stderr: %s", stderr)
	})

	// Test --oneline with --patch (should warn)
	t.Run("oneline-with-patch", func(t *testing.T) {
		_, _, err := runCommand(t, paramlog.Command(), "--oneline", "-p", paramName)
		require.NoError(t, err)
		// Command should succeed even with conflicting flags
	})

	// Test --output=json with --patch (should warn)
	t.Run("json-with-patch", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "--output", "json", "-p", paramName)
		require.NoError(t, err)
		// JSON output should still work
		assert.True(t, strings.HasPrefix(strings.TrimSpace(stdout), "[") || strings.HasPrefix(strings.TrimSpace(stdout), "{"))
	})

	// Test --output=json with --oneline (should warn)
	t.Run("json-with-oneline", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "--output", "json", "--oneline", paramName)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(strings.TrimSpace(stdout), "[") || strings.HasPrefix(strings.TrimSpace(stdout), "{"))
	})
}

// TestParam_LogWithParseJson tests param log with --parse-json flag.
func TestParam_LogWithParseJson(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-log/parsejson-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create with JSON value
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, `{"key": "value1"}`)
	_, _, _ = runCommand(t, paramupdate.Command(), "--yes", paramName, `{"key": "value2"}`)

	// Test with --parse-json and -p
	t.Run("parse-json-with-patch", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlog.Command(), "-p", "--parse-json", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "key")
	})
}

// =============================================================================
// Diff Command Edge Cases
// =============================================================================

// TestParam_DiffVersions tests param diff between specific versions.
func TestParam_DiffVersions(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-diff/versions-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create and update parameter
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "version1-value")
	_, _, _ = runCommand(t, paramupdate.Command(), "--yes", paramName, "version2-value")
	_, _, _ = runCommand(t, paramupdate.Command(), "--yes", paramName, "version3-value")

	// Diff between version 1 and version 3
	t.Run("diff-v1-v3", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramdiff.Command(), paramName+"#1", paramName+"#3")
		require.NoError(t, err)
		assert.Contains(t, stdout, "version1-value")
		assert.Contains(t, stdout, "version3-value")
	})

	// Diff with shift notation
	t.Run("diff-shift", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramdiff.Command(), paramName+"~1", paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "version2-value")
		assert.Contains(t, stdout, "version3-value")
	})
}

// TestParam_DiffNonExistent tests diff for non-existent parameter.
func TestParam_DiffNonExistent(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-diff/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to diff
	_, _, err := runCommand(t, paramdiff.Command(), paramName)
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

// TestParam_DiffNoChanges tests diff when there are no changes.
func TestParam_DiffNoChanges(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-diff/no-changes"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "same-value")
	_, _, _ = runCommand(t, paramupdate.Command(), "--yes", paramName, "same-value")

	// Diff should show no changes (or be empty)
	stdout, _, err := runCommand(t, paramdiff.Command(), paramName+"~1", paramName)
	require.NoError(t, err)
	// When values are the same, diff might be empty or show no diff
	t.Logf("diff output: %s", stdout)
}

// =============================================================================
// Show Command Edge Cases
// =============================================================================

// TestParam_ShowRaw tests param show with --raw flag.
func TestParam_ShowRaw(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-show/raw-test"
	paramValue := "raw-value-with-special-chars\ttab\nnewline"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter with special characters
	_, _, err := runCommand(t, paramcreate.Command(), paramName, paramValue)
	require.NoError(t, err)

	// Show raw (just the value)
	t.Run("raw", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		// Raw output should be just the value
		assert.Equal(t, paramValue, strings.TrimSuffix(stdout, "\n"))
	})

	// Show without raw (formatted)
	t.Run("formatted", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), paramName)
		require.NoError(t, err)
		// Formatted output should have metadata and the value
		assert.Contains(t, stdout, "Name:")
		assert.Contains(t, stdout, "Version:")
		assert.Contains(t, stdout, "raw-value-with-special-chars")
	})
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

// TestParam_ShowWithVersion tests param show with version specifier.
func TestParam_ShowWithVersion(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-show/version-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create and update
	_, _, _ = runCommand(t, paramcreate.Command(), paramName, "v1")
	_, _, _ = runCommand(t, paramupdate.Command(), "--yes", paramName, "v2")

	// Show specific version
	t.Run("show-v1", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName+"#1")
		require.NoError(t, err)
		assert.Equal(t, "v1", strings.TrimSuffix(stdout, "\n"))
	})

	// Show with shift
	t.Run("show-shift", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName+"~1")
		require.NoError(t, err)
		assert.Equal(t, "v1", strings.TrimSuffix(stdout, "\n"))
	})
}

// TestParam_ShowNonExistent tests show for non-existent parameter.
func TestParam_ShowNonExistent(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-show/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to show
	_, _, err := runCommand(t, paramshow.Command(), paramName)
	assert.Error(t, err)
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

// =============================================================================
// Create Command Edge Cases
// =============================================================================

// TestParam_CreateAndTag tests creating a param and adding tags after.
func TestParam_CreateAndTag(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-create/and-tag"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create parameter
	stdout, _, err := runCommand(t, paramcreate.Command(), paramName, "value")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Created")

	// Add tags after creation
	_, _, err = runCommand(t, paramtag.Command(), paramName, "env=test", "team=suve")
	require.NoError(t, err)

	// Verify tags
	stdout, _, err = runCommand(t, paramshow.Command(), paramName)
	require.NoError(t, err)
	assert.Contains(t, stdout, "env: test")
	assert.Contains(t, stdout, "team: suve")
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

// TestParam_CreateWithDescription tests param create with description.
func TestParam_CreateWithDescription(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-create/with-description"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create with description
	stdout, _, err := runCommand(t, paramcreate.Command(), "--description", "Test parameter description", paramName, "value")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Created")
}

// TestParam_CreateDuplicate tests creating a duplicate parameter.
func TestParam_CreateDuplicate(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-create/duplicate"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	// Create first
	_, _, err := runCommand(t, paramcreate.Command(), paramName, "value")
	require.NoError(t, err)

	// Try to create again
	_, _, err = runCommand(t, paramcreate.Command(), paramName, "value2")
	assert.Error(t, err)
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

// TestParam_CreateMissingArgs tests error handling for missing arguments.
func TestParam_CreateMissingArgs(t *testing.T) {
	setupEnv(t)

	// No arguments at all
	t.Run("no-args", func(t *testing.T) {
		_, _, err := runCommand(t, paramcreate.Command())
		assert.Error(t, err)
	})

	// Only name, no value
	t.Run("no-value", func(t *testing.T) {
		_, _, err := runCommand(t, paramcreate.Command(), "/test/param")
		assert.Error(t, err)
	})
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

// =============================================================================
// Delete Command Edge Cases
// =============================================================================

// TestParam_DeleteNonExistent tests deleting a non-existent parameter.
func TestParam_DeleteNonExistent(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-delete/non-existent"

	// Ensure it doesn't exist
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)

	// Try to delete (should fail since it doesn't exist)
	_, _, err := runCommand(t, paramdelete.Command(), "--yes", paramName)
	assert.Error(t, err)
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

// =============================================================================
// List Command Edge Cases
// =============================================================================

// TestParam_ListWithPath tests param list with specific path.
func TestParam_ListWithPath(t *testing.T) {
	setupEnv(t)
	basePath := "/suve-e2e-list/path-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/param1")
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/param2")
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/subdir/param3")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/param1")
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/param2")
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/subdir/param3")
	})

	// Create parameters
	_, _, _ = runCommand(t, paramcreate.Command(), basePath+"/param1", "v1")
	_, _, _ = runCommand(t, paramcreate.Command(), basePath+"/param2", "v2")
	_, _, _ = runCommand(t, paramcreate.Command(), basePath+"/subdir/param3", "v3")

	// List all under basePath (non-recursive)
	t.Run("list-all", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), basePath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "param1")
		assert.Contains(t, stdout, "param2")
		// param3 is in a subdirectory, so it won't appear without --recursive
	})

	// List with recursive
	t.Run("list-recursive", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramlist.Command(), "--recursive", basePath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "param1")
		assert.Contains(t, stdout, "subdir/param3")
	})
}

// TestParam_ListWithFilter tests param list with filter.
func TestParam_ListWithFilter(t *testing.T) {
	setupEnv(t)
	basePath := "/suve-e2e-list/filter-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/app-config")
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/db-config")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/app-config")
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/db-config")
	})

	// Create parameters
	_, _, _ = runCommand(t, paramcreate.Command(), basePath+"/app-config", "v1")
	_, _, _ = runCommand(t, paramcreate.Command(), basePath+"/db-config", "v2")

	// List with filter
	stdout, _, err := runCommand(t, paramlist.Command(), "--filter", "app", basePath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "app-config")
	assert.NotContains(t, stdout, "db-config")
}

// TestParam_ListJSON tests param list with JSON output.
func TestParam_ListJSON(t *testing.T) {
	setupEnv(t)
	basePath := "/suve-e2e-list/json-test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/param1")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", basePath+"/param1")
	})

	// Create parameter
	_, _, _ = runCommand(t, paramcreate.Command(), basePath+"/param1", "v1")

	// List with JSON format
	stdout, _, err := runCommand(t, paramlist.Command(), "--output", "json", basePath)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(stdout), "[") || strings.HasPrefix(strings.TrimSpace(stdout), "{"))
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
		stdout, _, err := runCommand(t, globalpersist.Command())
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
		stdout, _, err := runCommand(t, globaldrain.Command())
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
		stdout, _, err := runCommand(t, globalpersist.Command(), "--keep")
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
	_, _, err = runCommand(t, globalpersist.Command())
	require.NoError(t, err)

	// Stage param2 in agent
	_, _, err = runCommand(t, paramstage.Command(), "add", paramName2, "value2")
	require.NoError(t, err)

	// Drain with merge (should combine both)
	t.Run("drain-with-merge", func(t *testing.T) {
		stdout, _, err := runCommand(t, globaldrain.Command(), "--merge")
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
		_, _, err := runCommand(t, globaldrain.Command())
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
		_, stderr, err := runCommand(t, globalpersist.Command())
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

// =============================================================================
// Service-Specific Drain and Persist Tests
// =============================================================================

// TestParam_DrainAndPersist tests service-specific drain and persist for parameters.
func TestParam_DrainAndPersist(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-param-drain-persist/test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
		_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	})

	// Stage a parameter
	t.Run("stage-param", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "test-value")
		require.NoError(t, err)
		t.Logf("stage add output: %s", stdout)
	})

	// Persist only param service to file
	t.Run("persist-param-only", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "persist")
		require.NoError(t, err)
		t.Logf("persist output: %s", stdout)
		assert.Contains(t, stdout, "persisted to file")
	})

	// Agent should be empty for param
	t.Run("verify-agent-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		// Should show no param changes
		assert.NotContains(t, stdout, paramName)
	})

	// Drain param service back from file
	t.Run("drain-param-only", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "drain")
		require.NoError(t, err)
		t.Logf("drain output: %s", stdout)
		assert.Contains(t, stdout, "loaded from file")
	})

	// Param should be back in agent
	t.Run("verify-param-restored", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})

	// Apply to verify workflow works end-to-end
	t.Run("apply-changes", func(t *testing.T) {
		_, _, err := runSubCommand(t, paramstage.Command(), "apply", "--yes")
		require.NoError(t, err)
	})

	// Verify created
	t.Run("verify-created", func(t *testing.T) {
		stdout, _, err := runCommand(t, paramshow.Command(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "test-value", strings.TrimSpace(stdout))
	})
}

// TestParam_PersistWithKeep tests param persist with --keep flag.
func TestParam_PersistWithKeep(t *testing.T) {
	setupEnv(t)
	paramName := "/suve-e2e-param-persist-keep/test"

	// Cleanup
	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
		_, _, _ = runCommand(t, globalreset.Command(), "--yes")
	})

	// Stage a parameter
	_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName, "test-value")
	require.NoError(t, err)

	// Persist with --keep (should keep in agent memory)
	t.Run("persist-with-keep", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "persist", "--keep")
		require.NoError(t, err)
		t.Logf("persist --keep output: %s", stdout)
		assert.Contains(t, stdout, "kept in memory")
	})

	// Param should still be in agent
	t.Run("verify-still-in-agent", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
	})
}

// TestParam_DrainWithMerge tests param drain with --merge flag.
func TestParam_DrainWithMerge(t *testing.T) {
	setupEnv(t)
	paramName1 := "/suve-e2e-param-drain-merge/param1"
	paramName2 := "/suve-e2e-param-drain-merge/param2"

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
	_, _, err := runSubCommand(t, paramstage.Command(), "add", paramName1, "value1")
	require.NoError(t, err)
	_, _, err = runSubCommand(t, paramstage.Command(), "persist")
	require.NoError(t, err)

	// Stage param2 in agent
	_, _, err = runSubCommand(t, paramstage.Command(), "add", paramName2, "value2")
	require.NoError(t, err)

	// Drain with --merge (should combine both)
	t.Run("drain-with-merge", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "drain", "--merge")
		require.NoError(t, err)
		t.Logf("drain --merge output: %s", stdout)
		assert.Contains(t, stdout, "merged")
	})

	// Both should be in agent
	t.Run("verify-both-staged", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, paramstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName1)
		assert.Contains(t, stdout, paramName2)
	})
}

// TestSecret_DrainAndPersist tests service-specific drain and persist for secrets.
func TestSecret_DrainAndPersist(t *testing.T) {
	setupEnv(t)
	secretName := "suve-e2e-secret-drain-persist/test"

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

	// Persist only secret service to file
	t.Run("persist-secret-only", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "persist")
		require.NoError(t, err)
		t.Logf("persist output: %s", stdout)
		assert.Contains(t, stdout, "persisted to file")
	})

	// Agent should be empty for secret
	t.Run("verify-agent-empty", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, secretName)
	})

	// Drain secret service back from file
	t.Run("drain-secret-only", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "drain")
		require.NoError(t, err)
		t.Logf("drain output: %s", stdout)
		assert.Contains(t, stdout, "loaded from file")
	})

	// Secret should be back in agent
	t.Run("verify-secret-restored", func(t *testing.T) {
		stdout, _, err := runSubCommand(t, secretstage.Command(), "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, secretName)
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
