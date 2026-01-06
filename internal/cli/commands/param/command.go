package param

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/param/create"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/param/delete"
	"github.com/mpyw/suve/internal/cli/commands/param/diff"
	"github.com/mpyw/suve/internal/cli/commands/param/list"
	"github.com/mpyw/suve/internal/cli/commands/param/log"
	"github.com/mpyw/suve/internal/cli/commands/param/show"
	"github.com/mpyw/suve/internal/cli/commands/param/tag"
	"github.com/mpyw/suve/internal/cli/commands/param/untag"
	"github.com/mpyw/suve/internal/cli/commands/param/update"
)

// Command returns the param command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "param",
		Aliases: []string{"ssm", "ps"},
		Usage:   "Interact with AWS Systems Manager Parameter Store",
		Commands: []*cli.Command{
			show.Command(),
			log.Command(),
			diff.Command(),
			list.Command(),
			create.Command(),
			update.Command(),
			paramdelete.Command(),
			tag.Command(),
			untag.Command(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
