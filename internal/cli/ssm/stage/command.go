// Package stage provides the SSM stage subcommand for staging operations.
package stage

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/ssm/strategy"
	"github.com/mpyw/suve/internal/stage/stagerunner"
)

var config = stagerunner.CommandConfig{
	ServiceName:          "ssm",
	ItemName:             "parameter",
	Factory:              strategy.Factory,
	FactoryWithoutClient: strategy.FactoryWithoutClient,
}

// Command returns the stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "stage",
		Usage: "Staging operations for SSM parameters",
		Description: `Stage changes locally before pushing to AWS.

Use 'suve ssm stage edit' to edit and stage a parameter.
Use 'suve ssm stage delete' to stage a parameter for deletion.
Use 'suve ssm stage status' to view staged changes.
Use 'suve ssm stage diff' to see differences between staged and AWS values.
Use 'suve ssm stage push' to apply staged changes to AWS.
Use 'suve ssm stage reset' to unstage or restore from a version.`,
		Commands: []*cli.Command{
			stagerunner.NewEditCommand(config),
			stagerunner.NewDeleteCommand(config),
			stagerunner.NewStatusCommand(config),
			stagerunner.NewDiffCommand(config),
			stagerunner.NewPushCommand(config),
			stagerunner.NewResetCommand(config),
		},
	}
}
