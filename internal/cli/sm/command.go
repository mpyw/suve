package sm

import "github.com/urfave/cli/v2"

// Command returns the sm command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "sm",
		Aliases: []string{"secret"},
		Usage:   "Interact with AWS Secrets Manager",
		Subcommands: []*cli.Command{
			showCommand(),
			catCommand(),
			logCommand(),
			diffCommand(),
			lsCommand(),
			createCommand(),
			setCommand(),
			rmCommand(),
			restoreCommand(),
		},
	}
}
