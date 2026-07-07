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

// nounSecret is the command name / noun used across the Secrets Manager commands.
const nounSecret = "secret"

// Command returns the secret command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    nounSecret,
		Aliases: []string{"sm", "secretsmanager"},
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
