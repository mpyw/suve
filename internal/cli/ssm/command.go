package ssm

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/ssm/cat"
	"github.com/mpyw/suve/internal/cli/ssm/diff"
	"github.com/mpyw/suve/internal/cli/ssm/edit"
	"github.com/mpyw/suve/internal/cli/ssm/log"
	"github.com/mpyw/suve/internal/cli/ssm/ls"
	"github.com/mpyw/suve/internal/cli/ssm/push"
	"github.com/mpyw/suve/internal/cli/ssm/reset"
	"github.com/mpyw/suve/internal/cli/ssm/rm"
	"github.com/mpyw/suve/internal/cli/ssm/set"
	"github.com/mpyw/suve/internal/cli/ssm/show"
	"github.com/mpyw/suve/internal/cli/ssm/status"
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
			rm.Command(),
			edit.Command(),
			status.Command(),
			push.Command(),
			reset.Command(),
		},
	}
}
