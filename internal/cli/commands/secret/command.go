// Package secret provides CLI commands for AWS Secrets Manager.
package secret

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/secret/delete"
	"github.com/mpyw/suve/internal/cli/commands/secret/restore"
	"github.com/mpyw/suve/internal/cli/commands/secret/update"
)

// Command returns the secret command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "secret",
		Aliases: []string{"sm"},
		Usage:   "Interact with AWS Secrets Manager",
		Commands: []*cli.Command{
			ShowCommand(),
			LogCommand(),
			DiffCommand(),
			ListCommand(),
			create.Command(),
			update.Command(),
			secretdelete.Command(),
			restore.Command(),
			TagCommand(),
			UntagCommand(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
