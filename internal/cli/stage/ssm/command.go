// Package ssm provides the SSM stage subcommand for staging operations.
package ssm

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/internal"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/runner"
)

var config = runner.CommandConfig{
	ServiceName:   "ssm",
	ItemName:      "parameter",
	Factory:       staging.SSMFactory,
	ParserFactory: staging.SSMParserFactory,
}

// Command returns the SSM stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "ssm",
		Aliases: []string{"ps", "param"},
		Usage:   "Staging operations for SSM parameters",
		Description: `Stage changes locally before pushing to AWS.

Use 'suve stage ssm add' to create and stage a new parameter.
Use 'suve stage ssm edit' to edit and stage an existing parameter.
Use 'suve stage ssm delete' to stage a parameter for deletion.
Use 'suve stage ssm status' to view staged parameter changes.
Use 'suve stage ssm diff' to see differences between staged and AWS values.
Use 'suve stage ssm push' to apply staged parameter changes to AWS.
Use 'suve stage ssm reset' to unstage or restore from a version.`,
		Commands: []*cli.Command{
			runner.NewAddCommand(config),
			runner.NewEditCommand(config),
			runner.NewDeleteCommand(config),
			runner.NewStatusCommand(config),
			runner.NewDiffCommand(config),
			runner.NewPushCommand(config),
			runner.NewResetCommand(config),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
