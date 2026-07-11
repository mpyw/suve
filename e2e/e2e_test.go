//go:build e2e

// Package e2e contains end-to-end tests for the suve CLI.
//
// These tests run against a real AWS-compatible service (localstack) and verify
// the complete workflow of each command using the actual CLI commands.
//
// Run with: make e2e
//
// Environment variables:
//   - SUVE_LOCALSTACK_ENDPOINT: Full localstack URL (default:
//     http://localhost:4566). Host runs use the default; the in-container
//     (compose.test.yaml) runner points it at http://localstack:4566.
package e2e_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// TestMain sets up the environment before running tests.
func TestMain(m *testing.M) {
	// Create an isolated temp directory. The working staging area lives under
	// HOME (set per-test via setupTempHome) and export/import files are written
	// to per-test t.TempDir()s, but keep the platform's runtime-dir env vars
	// pinned to a short, isolated path for consistency.
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
	// Single full-URL knob. Host/manual runs get the default localhost URL; the
	// in-container (compose.test.yaml) runner sets it to http://localstack:4566
	// to reach the emulator by service name over the closed compose network.
	if endpoint := os.Getenv("SUVE_LOCALSTACK_ENDPOINT"); endpoint != "" {
		return endpoint
	}

	return "http://localhost:4566"
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
//
// It uses NewWorkingStore (not NewStore) so the read path shares the exact key
// resolution the CLI uses when it writes: SUVE_STAGING_KEY env var -> OS
// keychain -> plaintext. Under the Dockerized runner SUVE_STAGING_KEY is set,
// so the working store is encrypted and this must decrypt with the same key; on
// keyless/no-keychain environments both sides fall back to plaintext.
func newStore() *file.Store {
	s, err := file.NewWorkingStore(provider.AWSScope("000000000000", "us-east-1"))
	if err != nil {
		panic(err)
	}

	return s
}

// newStoreForAccount creates a staging store for a specific account and region.
func newStoreForAccount(accountID, region string) *file.Store {
	s, err := file.NewWorkingStore(provider.AWSScope(accountID, region))
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

// runCommandWithStdin executes a CLI command with a custom stdin (e.g. for
// --value-stdin) and returns stdout, stderr, and error.
func runCommandWithStdin(
	t *testing.T, cmd *cli.Command, stdin io.Reader, args ...string,
) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Reader:    stdin,
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{cmd},
	}

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
