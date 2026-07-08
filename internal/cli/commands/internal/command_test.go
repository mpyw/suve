package internal_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// TestCommandNotFound mutates the process-wide cli.OsExiter, so it is not
// parallel; it saves and restores the original.
//
//nolint:paralleltest // mutates the process-wide cli.OsExiter
func TestCommandNotFound(t *testing.T) {
	origExiter := cli.OsExiter

	t.Cleanup(func() { cli.OsExiter = origExiter })

	t.Run("outputs unknown command message and exits non-zero", func(t *testing.T) {
		var (
			called  bool
			gotCode int
		)

		cli.OsExiter = func(code int) { called, gotCode = true, code }

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		cmd := &cli.Command{
			Name:      "test",
			Writer:    stdout,
			ErrWriter: stderr,
			Commands: []*cli.Command{
				{Name: "subcommand", Usage: "A valid subcommand"},
			},
		}

		cliinternal.CommandNotFound(context.Background(), cmd, "unknown-cmd")

		// Should output the unknown command message to stderr (via output.Printf which uses ErrWriter)
		output := stdout.String() + stderr.String()
		assert.Contains(t, output, "Unknown command: unknown-cmd")

		// And it must exit non-zero so scripts/CI do not treat a typo as success.
		assert.True(t, called, "CommandNotFound must exit")
		assert.Equal(t, cliinternal.CommandNotFoundExitCode, gotCode)
		assert.NotZero(t, gotCode)
	})

	t.Run("shows help when ErrWriter is nil", func(t *testing.T) {
		cli.OsExiter = func(int) {} // swallow the exit

		stdout := &bytes.Buffer{}

		cmd := &cli.Command{
			Name:      "test",
			Writer:    stdout,
			ErrWriter: nil, // Fallback to Writer
			Commands: []*cli.Command{
				{Name: "subcommand", Usage: "A valid subcommand"},
			},
		}

		cliinternal.CommandNotFound(context.Background(), cmd, "foo")

		// Should output to Writer as fallback
		assert.Contains(t, stdout.String(), "Unknown command: foo")
	})
}
