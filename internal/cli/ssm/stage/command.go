// Package stage provides the SSM stage subcommand for staging operations.
package stage

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/staging/runner"
	"github.com/mpyw/suve/internal/staging/ssm"
)

var config = runner.CommandConfig{
	ServiceName:   "ssm",
	ItemName:      "parameter",
	Factory:       ssm.Factory,
	ParserFactory: ssm.ParserFactory,
}

// Command returns the stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "stage",
		Usage: "Staging operations for SSM parameters",
		Description: `Stage changes locally before pushing to AWS.

Use 'suve ssm stage add' to create and stage a new parameter.
Use 'suve ssm stage edit' to edit and stage an existing parameter.
Use 'suve ssm stage delete' to stage a parameter for deletion.
Use 'suve ssm stage status' to view staged changes.
Use 'suve ssm stage diff' to see differences between staged and AWS values.
Use 'suve ssm stage push' to apply staged changes to AWS.
Use 'suve ssm stage reset' to unstage or restore from a version.`,
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
