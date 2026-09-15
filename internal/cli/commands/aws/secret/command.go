// Package secret provides CLI commands for AWS Secrets Manager.
//
// command.go is this package's subject: the secret command group it assembles,
// together with the vocabulary its sibling files share. Hence core.
//
//declscope:core
package secret

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/aws/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/aws/secret/delete"
	"github.com/mpyw/suve/internal/cli/commands/aws/secret/restore"
	"github.com/mpyw/suve/internal/cli/commands/aws/secret/update"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// nounSecret is the command name / noun used across the Secrets Manager commands.
//
//declscope:package // tag.go and untag.go name their commands with it
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
