// Package param provides CLI commands for AWS SSM Parameter Store.
package param

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// Command returns the param command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "param",
		Aliases: []string{"ssm", "ps"},
		Usage:   "Interact with AWS Systems Manager Parameter Store",
		Commands: []*cli.Command{
			ShowCommand(),
			LogCommand(),
			DiffCommand(),
			ListCommand(),
			CreateCommand(),
			UpdateCommand(),
			DeleteCommand(),
			TagCommand(),
			UntagCommand(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
