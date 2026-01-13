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
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/agent"
	"github.com/mpyw/suve/internal/staging/store/agent/daemon"
)

// testDaemon is the shared staging agent daemon for all E2E tests.
var testDaemon *daemon.Runner

// TestMain sets up the staging agent daemon before running tests.
func TestMain(m *testing.M) {
	// Create isolated temp directory for socket path
	// Use /tmp directly to avoid socket path length limit issues on macOS
	// (macOS has a 104-byte limit for Unix socket paths)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-e2e-")
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

	// Start daemon with error channel (localstack uses account "000000000000" and region "us-east-1")
	testDaemon = daemon.NewRunner("000000000000", "us-east-1", agent.DaemonOptions()...)
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
	// Use same account/region as the daemon
	launcher := daemon.NewLauncher("000000000000", "us-east-1", daemon.WithAutoStartDisabled())
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

// newStoreForAccount creates a staging store for a specific account and region.
// Used for testing error cases when daemon is not running for that account.
func newStoreForAccount(accountID, region string) store.AgentStore {
	return agent.NewStore(accountID, region)
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
