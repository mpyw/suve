// Package internal provides shared utilities for CLI commands.
package internal

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// CommandNotFound is a shared handler for unknown subcommands.
// It displays the command help and an error message.
func CommandNotFound(ctx context.Context, cmd *cli.Command, command string) {
	_ = cli.ShowCommandHelp(ctx, cmd, "")
	w := cmd.Root().ErrWriter
	if w == nil {
		w = cmd.Root().Writer
	}
	_, _ = fmt.Fprintf(w, "\nUnknown command: %s\n", command)
}
