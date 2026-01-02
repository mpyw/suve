package sm

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/sm/cat"
	"github.com/mpyw/suve/internal/cli/commands/sm/create"
	smdelete "github.com/mpyw/suve/internal/cli/commands/sm/delete"
	"github.com/mpyw/suve/internal/cli/commands/sm/diff"
	"github.com/mpyw/suve/internal/cli/commands/sm/log"
	"github.com/mpyw/suve/internal/cli/commands/sm/ls"
	"github.com/mpyw/suve/internal/cli/commands/sm/restore"
	"github.com/mpyw/suve/internal/cli/commands/sm/show"
	"github.com/mpyw/suve/internal/cli/commands/sm/update"
)

// Command returns the sm command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "sm",
		Aliases: []string{"secret"},
		Usage:   "Interact with AWS Secrets Manager",
		Commands: []*cli.Command{
			show.Command(),
			cat.Command(),
			log.Command(),
			diff.Command(),
			ls.Command(),
			create.Command(),
			update.Command(),
			smdelete.Command(),
			restore.Command(),
			{
				Name:   "set",
				Hidden: true,
				Action: func(_ context.Context, _ *cli.Command) error {
					return fmt.Errorf(`'suve sm set' is not available

Secrets Manager distinguishes between creating and updating secrets:
  suve sm create <name> <value>   Create a new secret
  suve sm update <name> <value>   Update an existing secret

Unlike SSM Parameter Store, these operations use different AWS APIs`)
				},
			},
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
