package ssm

import "github.com/urfave/cli/v2"

// Command returns the ssm command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "ssm",
		Aliases: []string{"ps", "param"},
		Usage:   "Interact with AWS Systems Manager Parameter Store",
		Subcommands: []*cli.Command{
			showCommand(),
			catCommand(),
			logCommand(),
			diffCommand(),
			lsCommand(),
			setCommand(),
			rmCommand(),
		},
	}
}
