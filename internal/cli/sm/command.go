package sm

import (
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/cli/sm/cat"
	"github.com/mpyw/suve/internal/cli/sm/create"
	"github.com/mpyw/suve/internal/cli/sm/diff"
	"github.com/mpyw/suve/internal/cli/sm/log"
	"github.com/mpyw/suve/internal/cli/sm/ls"
	"github.com/mpyw/suve/internal/cli/sm/restore"
	"github.com/mpyw/suve/internal/cli/sm/rm"
	"github.com/mpyw/suve/internal/cli/sm/set"
	"github.com/mpyw/suve/internal/cli/sm/show"
)

// Command returns the sm command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "sm",
		Aliases: []string{"secret"},
		Usage:   "Interact with AWS Secrets Manager",
		Subcommands: []*cli.Command{
			show.Command(),
			cat.Command(),
			log.Command(),
			diff.Command(),
			ls.Command(),
			create.Command(),
			set.Command(),
			rm.Command(),
			restore.Command(),
		},
	}
}
