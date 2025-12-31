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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	smcat "github.com/mpyw/suve/internal/cli/sm/cat"
	smcreate "github.com/mpyw/suve/internal/cli/sm/create"
	smdiff "github.com/mpyw/suve/internal/cli/sm/diff"
	smlog "github.com/mpyw/suve/internal/cli/sm/log"
	smls "github.com/mpyw/suve/internal/cli/sm/ls"
	smrestore "github.com/mpyw/suve/internal/cli/sm/restore"
	smrm "github.com/mpyw/suve/internal/cli/sm/rm"
	smshow "github.com/mpyw/suve/internal/cli/sm/show"
	smupdate "github.com/mpyw/suve/internal/cli/sm/update"
	ssmcat "github.com/mpyw/suve/internal/cli/ssm/cat"
	ssmdiff "github.com/mpyw/suve/internal/cli/ssm/diff"
	ssmlog "github.com/mpyw/suve/internal/cli/ssm/log"
	ssmls "github.com/mpyw/suve/internal/cli/ssm/ls"
	ssmrm "github.com/mpyw/suve/internal/cli/ssm/rm"
	ssmset "github.com/mpyw/suve/internal/cli/ssm/set"
	ssmshow "github.com/mpyw/suve/internal/cli/ssm/show"
)

func getEndpoint() string {
	port := os.Getenv("SUVE_LOCALSTACK_EXTERNAL_PORT")
	if port == "" {
		port = "4566"
	}
	return fmt.Sprintf("http://127.0.0.1:%s", port)
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

// runCommand executes a CLI command and returns stdout, stderr, and error.
func runCommand(t *testing.T, cmd *cli.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer
	app := &cli.App{
		Name:      "suve",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{cmd},
	}

	// Build full args: ["suve", "command-name", ...args]
	fullArgs := append([]string{"suve", cmd.Name}, args...)
	err = app.RunContext(t.Context(), fullArgs)

	return outBuf.String(), errBuf.String(), err
}

// TestSSM_FullWorkflow tests the complete SSM Parameter Store workflow:
// set → show → cat → update → log → diff → ls → rm → verify deletion
//
// This test creates a parameter, updates it, verifies version history,
// compares versions using diff, and cleans up by deleting.
func TestSSM_FullWorkflow(t *testing.T) {
	t.Parallel()
	setupEnv(t)
	paramName := "/suve-e2e-test/param"

	// Cleanup: delete parameter if it exists (ignore errors)
	_, _, _ = runCommand(t, ssmrm.Command(), paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, ssmrm.Command(), paramName)
	})

	// 1. Set parameter
	t.Run("set", func(t *testing.T) {
		stdout, _, err := runCommand(t, ssmset.Command(), paramName, "initial-value")
		require.NoError(t, err)
		t.Logf("set output: %s", stdout)
	})

	// 2. Show parameter
	t.Run("show", func(t *testing.T) {
		stdout, _, err := runCommand(t, ssmshow.Command(), paramName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "initial-value")
		t.Logf("show output: %s", stdout)
	})

	// 3. Cat parameter (raw output without trailing newline)
	t.Run("cat", func(t *testing.T) {
		stdout, _, err := runCommand(t, ssmcat.Command(), paramName)
		require.NoError(t, err)
		assert.Equal(t, "initial-value", stdout)
	})

	// 4. Update parameter
	t.Run("update", func(t *testing.T) {
		_, _, err := runCommand(t, ssmset.Command(), paramName, "updated-value")
		require.NoError(t, err)
	})

	// 5. Log (without patch)
	t.Run("log", func(t *testing.T) {
		stdout, _, err := runCommand(t, ssmlog.Command(), paramName)
		require.NoError(t, err)
		t.Logf("log output: %s", stdout)
	})

	// 6. Diff - Compare version 1 with version 2
	t.Run("diff", func(t *testing.T) {
		stdout, _, err := runCommand(t, ssmdiff.Command(), paramName+"#1", paramName+"#2")
		require.NoError(t, err)
		t.Logf("diff output: %s", stdout)
		assert.Contains(t, stdout, "-initial-value")
		assert.Contains(t, stdout, "+updated-value")
	})

	// 7. List
	t.Run("ls", func(t *testing.T) {
		stdout, _, err := runCommand(t, ssmls.Command(), "/suve-e2e-test/")
		require.NoError(t, err)
		assert.Contains(t, stdout, paramName)
		t.Logf("ls output: %s", stdout)
	})

	// 8. Delete
	t.Run("rm", func(t *testing.T) {
		_, _, err := runCommand(t, ssmrm.Command(), paramName)
		require.NoError(t, err)
	})

	// 9. Verify deletion
	t.Run("verify-deleted", func(t *testing.T) {
		_, _, err := runCommand(t, ssmshow.Command(), paramName)
		assert.Error(t, err, "expected error after deletion")
	})
}

// TestSM_FullWorkflow tests the complete Secrets Manager workflow:
// create → show → cat → update → log → diff → ls → rm → restore → verify → force-rm
//
// This test creates a secret, updates it, verifies version history using labels,
// compares versions using diff, tests soft delete with recovery, and cleans up
// with force delete.
func TestSM_FullWorkflow(t *testing.T) {
	t.Parallel()
	setupEnv(t)
	secretName := "suve-e2e-test/secret"

	// Cleanup: force delete secret if it exists (ignore errors)
	_, _, _ = runCommand(t, smrm.Command(), "-f", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, smrm.Command(), "-f", secretName)
	})

	// 1. Create secret
	t.Run("create", func(t *testing.T) {
		stdout, _, err := runCommand(t, smcreate.Command(), secretName, "initial-secret")
		require.NoError(t, err)
		t.Logf("create output: %s", stdout)
	})

	// 2. Show secret
	t.Run("show", func(t *testing.T) {
		stdout, _, err := runCommand(t, smshow.Command(), secretName)
		require.NoError(t, err)
		assert.Contains(t, stdout, "initial-secret")
		t.Logf("show output: %s", stdout)
	})

	// 3. Cat secret (raw output without trailing newline)
	t.Run("cat", func(t *testing.T) {
		stdout, _, err := runCommand(t, smcat.Command(), secretName)
		require.NoError(t, err)
		assert.Equal(t, "initial-secret", stdout)
	})

	// 4. Update secret
	t.Run("update", func(t *testing.T) {
		_, _, err := runCommand(t, smupdate.Command(), secretName, "updated-secret")
		require.NoError(t, err)
	})

	// 5. Log (without patch)
	t.Run("log", func(t *testing.T) {
		stdout, _, err := runCommand(t, smlog.Command(), secretName)
		require.NoError(t, err)
		t.Logf("log output: %s", stdout)
	})

	// 6. Diff - Compare AWSPREVIOUS with AWSCURRENT
	t.Run("diff", func(t *testing.T) {
		stdout, _, err := runCommand(t, smdiff.Command(), secretName+":AWSPREVIOUS", secretName+":AWSCURRENT")
		require.NoError(t, err)
		t.Logf("diff output: %s", stdout)
		assert.Contains(t, stdout, "-initial-secret")
		assert.Contains(t, stdout, "+updated-secret")
	})

	// 7. List
	t.Run("ls", func(t *testing.T) {
		stdout, _, err := runCommand(t, smls.Command())
		require.NoError(t, err)
		assert.Contains(t, stdout, secretName)
		t.Logf("ls output: %s", stdout)
	})

	// 8. Delete (with recovery window)
	t.Run("rm", func(t *testing.T) {
		_, _, err := runCommand(t, smrm.Command(), "--recovery-window", "7", secretName)
		require.NoError(t, err)
	})

	// 9. Restore
	t.Run("restore", func(t *testing.T) {
		_, _, err := runCommand(t, smrestore.Command(), secretName)
		require.NoError(t, err)
	})

	// 10. Verify restored
	t.Run("verify-restored", func(t *testing.T) {
		_, _, err := runCommand(t, smshow.Command(), secretName)
		require.NoError(t, err)
	})

	// 11. Final cleanup (force delete)
	t.Run("force-rm", func(t *testing.T) {
		_, _, err := runCommand(t, smrm.Command(), "-f", secretName)
		require.NoError(t, err)
	})
}
