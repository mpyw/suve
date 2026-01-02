// Package param provides the param stage subcommand for staging operations.
package param

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/runner"
)

var config = runner.CommandConfig{
	ServiceName:   "param",
	ItemName:      "parameter",
	Factory:       staging.ParamFactory,
	ParserFactory: staging.ParamParserFactory,
}

// Command returns the param stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "param",
		Aliases: []string{"ssm", "ps"},
		Usage:   "Staging operations for SSM parameters",
		Description: `Stage changes locally before pushing to AWS.

Use 'suve stage param add' to create and stage a new parameter.
Use 'suve stage param edit' to edit and stage an existing parameter.
Use 'suve stage param delete' to stage a parameter for deletion.
Use 'suve stage param status' to view staged parameter changes.
Use 'suve stage param diff' to see differences between staged and AWS values.
Use 'suve stage param apply' to apply staged parameter changes to AWS.
Use 'suve stage param reset' to unstage or restore from a version.`,
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
