package internal_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

func TestCommandNotFound(t *testing.T) {
	t.Parallel()

	t.Run("outputs unknown command message", func(t *testing.T) {
		t.Parallel()

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
	})

	t.Run("shows help when ErrWriter is nil", func(t *testing.T) {
		t.Parallel()

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
