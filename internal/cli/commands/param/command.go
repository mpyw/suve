package param

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/param/delete"
	"github.com/mpyw/suve/internal/cli/commands/param/diff"
	"github.com/mpyw/suve/internal/cli/commands/param/log"
	"github.com/mpyw/suve/internal/cli/commands/param/ls"
	"github.com/mpyw/suve/internal/cli/commands/param/set"
	"github.com/mpyw/suve/internal/cli/commands/param/show"
)

// Command returns the ssm command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "param",
		Aliases: []string{"ssm", "ps"},
		Usage:   "Interact with AWS Systems Manager Parameter Store",
		Commands: []*cli.Command{
			show.Command(),
			log.Command(),
			diff.Command(),
			ls.Command(),
			set.Command(),
			paramdelete.Command(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
