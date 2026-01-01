// Package sm provides the SM stage subcommand for staging operations.
package sm

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/runner"
)

var config = runner.CommandConfig{
	ServiceName:   "sm",
	ItemName:      "secret",
	Factory:       staging.SMFactory,
	ParserFactory: staging.SMParserFactory,
}

// Command returns the SM stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "sm",
		Aliases: []string{"secret"},
		Usage:   "Staging operations for Secrets Manager",
		Description: `Stage changes locally before pushing to AWS.

Use 'suve stage sm add' to create and stage a new secret.
Use 'suve stage sm edit' to edit and stage an existing secret.
Use 'suve stage sm delete' to stage a secret for deletion.
Use 'suve stage sm status' to view staged secret changes.
Use 'suve stage sm diff' to see differences between staged and AWS values.
Use 'suve stage sm push' to apply staged secret changes to AWS.
Use 'suve stage sm reset' to unstage or restore from a version.`,
		Commands: []*cli.Command{
			runner.NewAddCommand(config),
			runner.NewEditCommand(config),
			runner.NewDeleteCommand(config),
			runner.NewStatusCommand(config),
			runner.NewDiffCommand(config),
			runner.NewPushCommand(config),
			runner.NewResetCommand(config),
		},
	}
}
