// Package stage provides the SM stage subcommand for staging operations.
package stage

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/sm/strategy"
	"github.com/mpyw/suve/internal/stage/stagerunner"
)

var config = stagerunner.CommandConfig{
	ServiceName:          "sm",
	ItemName:             "secret",
	Factory:              strategy.Factory,
	FactoryWithoutClient: strategy.FactoryWithoutClient,
}

// Command returns the stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "stage",
		Usage: "Staging operations for Secrets Manager",
		Description: `Stage changes locally before pushing to AWS.

Use 'suve sm stage edit' to edit and stage a secret.
Use 'suve sm stage delete' to stage a secret for deletion.
Use 'suve sm stage status' to view staged changes.
Use 'suve sm stage diff' to see differences between staged and AWS values.
Use 'suve sm stage push' to apply staged changes to AWS.
Use 'suve sm stage reset' to unstage or restore from a version.`,
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
