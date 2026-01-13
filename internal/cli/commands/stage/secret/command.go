// Package secret provides the secret stage subcommand for staging operations.
package secret

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

var config = stgcli.CommandConfig{
	CommandName:   "secret",
	ItemName:      "secret",
	Factory:       staging.SecretFactory,
	ParserFactory: staging.SecretParserFactory,
}

// Command returns the secret stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "secret",
		Aliases: []string{"sm"},
		Usage:   "Staging operations for Secrets Manager",
		Description: `Stage changes locally before applying to AWS.

Use 'suve stage secret add' to create and stage a new secret.
Use 'suve stage secret edit' to edit and stage an existing secret.
Use 'suve stage secret delete' to stage a secret for deletion.
Use 'suve stage secret status' to view staged secret changes.
Use 'suve stage secret diff' to see differences between staged and AWS values.
Use 'suve stage secret apply' to apply staged secret changes to AWS.
Use 'suve stage secret reset' to unstage or restore from a version.`,
		Commands: []*cli.Command{
			stgcli.NewAddCommand(config),
			stgcli.NewEditCommand(config),
			stgcli.NewDeleteCommand(config),
			stgcli.NewStatusCommand(config),
			stgcli.NewDiffCommand(config),
			stgcli.NewApplyCommand(config),
			stgcli.NewResetCommand(config),
			stgcli.NewTagCommand(config),
			stgcli.NewUntagCommand(config),
			stgcli.NewDrainCommand(config),
			stgcli.NewPersistCommand(config),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
