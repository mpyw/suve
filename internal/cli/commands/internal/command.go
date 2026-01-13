// Package internal provides shared utilities for CLI commands.
package internal

import (
	"context"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
)

// CommandNotFound is a shared handler for unknown subcommands.
// It displays the command help and an error message.
func CommandNotFound(_ context.Context, cmd *cli.Command, command string) {
	_ = cli.ShowSubcommandHelp(cmd)
	w := lo.CoalesceOrEmpty(cmd.Root().ErrWriter, cmd.Root().Writer)
	output.Printf(w, "\nUnknown command: %s\n", command)
}
