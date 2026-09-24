//go:build e2e

// The provider-neutral harness every suite drives is the unit this package is
// named for, and every suite file drives it.
//declscope:core
//declscope:package

// Package e2e contains end-to-end tests for the suve CLI.
//
// These tests run against real cloud-compatible emulators and verify the
// complete workflow of each command using the actual CLI commands. Each
// provider's setup lives in its own file (e.g. aws_test.go); this file holds
// only the provider-neutral harness.
package e2e_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
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

// setupTempHome sets up a temporary HOME directory for isolated staging tests.
func setupTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// testContext returns the context e2e helpers and cleanups pass to CLI commands
// and stores. Go cancels t.Context() just before t.Cleanup functions run, so a
// cleanup that deletes emulator resources under it would fail at once and leak
// leftovers into later tests. Once the test context is done, keep its values but
// drop the cancellation so cleanup work still reaches the emulator.
func testContext(t *testing.T) context.Context {
	t.Helper()

	ctx := t.Context()
	if ctx.Err() != nil {
		return context.WithoutCancel(ctx)
	}

	return ctx
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
	err = app.Run(testContext(t), fullArgs)

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
	err = app.Run(testContext(t), fullArgs)

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
	err = app.Run(testContext(t), fullArgs)

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
	err = app.Run(testContext(t), fullArgs)

	return outBuf.String(), errBuf.String(), err
}
