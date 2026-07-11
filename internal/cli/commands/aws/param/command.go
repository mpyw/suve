// Package param provides CLI commands for AWS SSM Parameter Store.
package param

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/aws/param/create"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/aws/param/delete"
	"github.com/mpyw/suve/internal/cli/commands/aws/param/update"
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
			create.Command(),
			update.Command(),
			paramdelete.Command(),
			TagCommand(),
			UntagCommand(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
