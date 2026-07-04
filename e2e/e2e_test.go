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
package e2e_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// TestMain sets up the environment before running tests.
func TestMain(m *testing.M) {
	// Create an isolated temp directory. The staging area and stash files live
	// under HOME (set per-test via setupTempHome), but keep the platform's
	// runtime-dir env vars pinned to a short, isolated path for consistency.
	// Use /tmp directly to avoid path length limit issues on macOS.
	tmpDir, err := os.MkdirTemp("/tmp", "suve-e2e-")
	if err != nil {
		output.Printf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	for _, key := range []string{"TMPDIR", "XDG_RUNTIME_DIR", "LOCALAPPDATA"} {
		if err := os.Setenv(key, tmpDir); err != nil {
			output.Printf(os.Stderr, "failed to set %s: %v\n", key, err)
			os.Exit(1)
		}
	}

	// Run tests
	code := m.Run()

	// Cleanup
	_ = os.RemoveAll(tmpDir)

	os.Exit(code)
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
func setupTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// newStore creates a new staging store for E2E tests.
// localstack uses account ID "000000000000" and region "us-east-1".
func newStore() *file.Store {
	s, err := file.NewStore("000000000000", "us-east-1")
	if err != nil {
		panic(err)
	}

	return s
}

// newStoreForAccount creates a staging store for a specific account and region.
func newStoreForAccount(accountID, region string) *file.Store {
	s, err := file.NewStore(accountID, region)
	if err != nil {
		panic(err)
	}

	return s
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

// runSubCommandWithStdin executes a subcommand with custom stdin for passphrase input.
func runSubCommandWithStdin(
	t *testing.T, parentCmd *cli.Command, stdin io.Reader, subCmdName string, args ...string,
) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Reader:    stdin,
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{parentCmd},
	}

	// Build full args: ["suve", "parent-name", "sub-name", ...args]
	fullArgs := append([]string{"suve", parentCmd.Name, subCmdName}, args...)
	err = app.Run(t.Context(), fullArgs)

	return outBuf.String(), errBuf.String(), err
}

// runStashSubCommandWithStdin executes a stash subcommand (e.g., "param stash pop") with custom stdin.
func runStashSubCommandWithStdin(
	t *testing.T, parentCmd *cli.Command, stdin io.Reader, stashSubCmd string, args ...string,
) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Reader:    stdin,
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{parentCmd},
	}

	// Build full args: ["suve", "parent-name", "stash", "stash-sub-cmd", ...args]
	fullArgs := append([]string{"suve", parentCmd.Name, "stash", stashSubCmd}, args...)
	err = app.Run(t.Context(), fullArgs)

	return outBuf.String(), errBuf.String(), err
}
