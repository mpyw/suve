//go:build e2e

//nolint:paralleltest,dogsled,gosec // E2E tests: sequential execution, cleanup, G101 false positive
package e2e_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmdparam "github.com/mpyw/suve/internal/cli/commands/aws/param"
	paramcreate "github.com/mpyw/suve/internal/cli/commands/aws/param/create"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/aws/param/delete"
	paramupdate "github.com/mpyw/suve/internal/cli/commands/aws/param/update"
	cmdsecret "github.com/mpyw/suve/internal/cli/commands/aws/secret"
	secretcreate "github.com/mpyw/suve/internal/cli/commands/aws/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/aws/secret/delete"
	secretupdate "github.com/mpyw/suve/internal/cli/commands/aws/secret/update"
)

// =============================================================================
// Interactive confirm/diff prompt tests
//
// The confirm and diff blocks in the destructive update/delete actions only run
// WITHOUT --yes, so the e2e suite (which otherwise always passes --yes) never
// reaches them. These tests drive those blocks by feeding "y\n"/"n\n" on stdin,
// asserting both that the prompt/diff renders and that the abort path leaves the
// value unmutated.
//
// The update actions read confirmation from the CLI's injected reader
// (internal.Stdin), so they use runCommandWithStdin. The delete actions read
// os.Stdin directly, so withOSStdin swaps it for the duration of the call. E2E
// tests are sequential (setupEnv uses t.Setenv, which forbids t.Parallel), so
// the process-global os.Stdin swap is safe here.
// =============================================================================

// withOSStdin temporarily replaces os.Stdin with a pipe preloaded with input,
// runs fn, then restores the original os.Stdin. Used for the delete actions,
// whose confirmation prompter reads os.Stdin directly.
func withOSStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, err = w.WriteString(input)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	orig := os.Stdin
	os.Stdin = r

	defer func() {
		os.Stdin = orig
		_ = r.Close()
	}()

	fn()
}

// TestAWSParam_UpdateConfirmPrompt drives the param update confirm+diff block
// (run without --yes) for both the confirm ("y") and abort ("n") paths.
func TestAWSParam_UpdateConfirmPrompt(t *testing.T) {
	setupEnv(t)

	paramName := "/suve-e2e-test/confirm/update-param"

	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	_, _, err := runCommand(t, paramcreate.Command(), paramName, "initial-value")
	require.NoError(t, err)

	// Confirm with "y": the diff+prompt renders and the update is applied.
	t.Run("confirm-yes", func(t *testing.T) {
		_, stderr, err := runCommandWithStdin(
			t, paramupdate.Command(), strings.NewReader("y\n"), paramName, "confirmed-value",
		)
		require.NoError(t, err)
		assert.Contains(t, stderr, "Update parameter")
		assert.Contains(t, stderr, "[y/N]")
		// Diff of old -> new value is shown before the prompt.
		assert.Contains(t, stderr, "-initial-value")
		assert.Contains(t, stderr, "+confirmed-value")

		stdout, _, err := runCommand(t, cmdparam.ShowCommand(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "confirmed-value", stdout)
	})

	// Abort with "n": the prompt renders but the value is left unchanged.
	t.Run("abort-no", func(t *testing.T) {
		_, stderr, err := runCommandWithStdin(
			t, paramupdate.Command(), strings.NewReader("n\n"), paramName, "aborted-value",
		)
		require.NoError(t, err)
		assert.Contains(t, stderr, "Update parameter")
		assert.Contains(t, stderr, "[y/N]")

		stdout, _, err := runCommand(t, cmdparam.ShowCommand(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "confirmed-value", stdout, "abort must not mutate the value")
	})
}

// TestAWSParam_DeleteConfirmPrompt drives the param delete confirm block (run
// without --yes) for both the abort ("n") and confirm ("y") paths.
func TestAWSParam_DeleteConfirmPrompt(t *testing.T) {
	setupEnv(t)

	paramName := "/suve-e2e-test/confirm/delete-param"

	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	_, _, err := runCommand(t, paramcreate.Command(), paramName, "delete-me")
	require.NoError(t, err)

	// Abort with "n": the current value + prompt render but nothing is deleted.
	t.Run("abort-no", func(t *testing.T) {
		var stderr string

		withOSStdin(t, "n\n", func() {
			_, stderr, err = runCommand(t, paramdelete.Command(), paramName)
		})
		require.NoError(t, err)
		assert.Contains(t, stderr, "delete-me", "current value should be shown before the prompt")
		assert.Contains(t, stderr, "permanently delete")
		assert.Contains(t, stderr, "[y/N]")

		// The parameter still exists.
		stdout, _, err := runCommand(t, cmdparam.ShowCommand(), "--raw", paramName)
		require.NoError(t, err)
		assert.Equal(t, "delete-me", stdout)
	})

	// Confirm with "y": the prompt renders and the parameter is deleted.
	t.Run("confirm-yes", func(t *testing.T) {
		var stderr string

		withOSStdin(t, "y\n", func() {
			_, stderr, err = runCommand(t, paramdelete.Command(), paramName)
		})
		require.NoError(t, err)
		assert.Contains(t, stderr, "permanently delete")
		assert.Contains(t, stderr, "[y/N]")

		// The parameter is gone.
		_, _, err := runCommand(t, cmdparam.ShowCommand(), paramName)
		assert.Error(t, err, "expected error after deletion")
	})
}

// TestAWSSecret_UpdateConfirmPrompt drives the secret update confirm+diff block
// (run without --yes) for both the confirm ("y") and abort ("n") paths.
func TestAWSSecret_UpdateConfirmPrompt(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-test/confirm/update-secret"

	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	_, _, err := runCommand(t, secretcreate.Command(), secretName, "initial-secret")
	require.NoError(t, err)

	// Confirm with "y": the diff+prompt renders and the update is applied.
	t.Run("confirm-yes", func(t *testing.T) {
		_, stderr, err := runCommandWithStdin(
			t, secretupdate.Command(), strings.NewReader("y\n"), secretName, "confirmed-secret",
		)
		require.NoError(t, err)
		assert.Contains(t, stderr, "Update secret")
		assert.Contains(t, stderr, "[y/N]")
		assert.Contains(t, stderr, "-initial-secret")
		assert.Contains(t, stderr, "+confirmed-secret")

		stdout, _, err := runCommand(t, cmdsecret.ShowCommand(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, "confirmed-secret", stdout)
	})

	// Abort with "n": the prompt renders but the value is left unchanged.
	t.Run("abort-no", func(t *testing.T) {
		_, stderr, err := runCommandWithStdin(
			t, secretupdate.Command(), strings.NewReader("n\n"), secretName, "aborted-secret",
		)
		require.NoError(t, err)
		assert.Contains(t, stderr, "Update secret")
		assert.Contains(t, stderr, "[y/N]")

		stdout, _, err := runCommand(t, cmdsecret.ShowCommand(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, "confirmed-secret", stdout, "abort must not mutate the value")
	})
}

// TestAWSSecret_DeleteConfirmPrompt drives the secret delete confirm block (run
// without --yes) for both the abort ("n") and confirm ("y") paths.
func TestAWSSecret_DeleteConfirmPrompt(t *testing.T) {
	setupEnv(t)

	secretName := "suve-e2e-test/confirm/delete-secret"

	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	_, _, err := runCommand(t, secretcreate.Command(), secretName, "delete-me-secret")
	require.NoError(t, err)

	// Abort with "n": the current value + prompt render but nothing is deleted.
	t.Run("abort-no", func(t *testing.T) {
		var stderr string

		withOSStdin(t, "n\n", func() {
			_, stderr, err = runCommand(t, secretdelete.Command(), secretName)
		})
		require.NoError(t, err)
		assert.Contains(t, stderr, "delete-me-secret", "current value should be shown before the prompt")
		assert.Contains(t, stderr, "permanently delete")
		assert.Contains(t, stderr, "[y/N]")

		// The secret still exists.
		stdout, _, err := runCommand(t, cmdsecret.ShowCommand(), "--raw", secretName)
		require.NoError(t, err)
		assert.Equal(t, "delete-me-secret", stdout)
	})

	// Confirm with "y": the prompt renders and the secret is scheduled for deletion.
	t.Run("confirm-yes", func(t *testing.T) {
		var stderr string

		withOSStdin(t, "y\n", func() {
			_, stderr, err = runCommand(t, secretdelete.Command(), "--force", secretName)
		})
		require.NoError(t, err)
		assert.Contains(t, stderr, "permanently delete")
		assert.Contains(t, stderr, "[y/N]")

		// The secret is gone.
		_, _, err := runCommand(t, cmdsecret.ShowCommand(), secretName)
		assert.Error(t, err, "expected error after deletion")
	})
}
