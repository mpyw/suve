// Package internal provides shared utilities for CLI commands.
package internal

import (
	"context"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
)

// CommandNotFoundExitCode is the process exit code produced for an unknown
// command. urfave/cli's CommandNotFound hook is a void func, so Command.Run
// returns nil after it runs and the process would otherwise exit 0 — making
// scripts/CI treat typos as success. The handlers exit non-zero explicitly.
const CommandNotFoundExitCode = 1

// CommandNotFound is a shared handler for unknown subcommands.
// It displays the command help and an error message, then exits non-zero.
func CommandNotFound(_ context.Context, cmd *cli.Command, command string) {
	_ = cli.ShowSubcommandHelp(cmd)
	w := lo.CoalesceOrEmpty(cmd.Root().ErrWriter, cmd.Root().Writer)
	output.Println(w, "")
	output.Warning(w, "Unknown command: %s", command)

	// urfave/cli's CommandNotFound is void, so Run returns nil; exit non-zero
	// here (via the overridable cli.OsExiter) so an unknown command fails.
	cli.OsExiter(CommandNotFoundExitCode)
}
