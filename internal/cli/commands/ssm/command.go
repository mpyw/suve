package ssm

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/ssm/cat"
	ssmdelete "github.com/mpyw/suve/internal/cli/commands/ssm/delete"
	"github.com/mpyw/suve/internal/cli/commands/ssm/diff"
	"github.com/mpyw/suve/internal/cli/commands/ssm/log"
	"github.com/mpyw/suve/internal/cli/commands/ssm/ls"
	"github.com/mpyw/suve/internal/cli/commands/ssm/set"
	"github.com/mpyw/suve/internal/cli/commands/ssm/show"
)

// Command returns the ssm command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "ssm",
		Aliases: []string{"ps", "param"},
		Usage:   "Interact with AWS Systems Manager Parameter Store",
		Commands: []*cli.Command{
			show.Command(),
			cat.Command(),
			log.Command(),
			diff.Command(),
			ls.Command(),
			set.Command(),
			ssmdelete.Command(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
