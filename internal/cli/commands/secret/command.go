package secret

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/secret/delete"
	"github.com/mpyw/suve/internal/cli/commands/secret/diff"
	"github.com/mpyw/suve/internal/cli/commands/secret/list"
	"github.com/mpyw/suve/internal/cli/commands/secret/log"
	"github.com/mpyw/suve/internal/cli/commands/secret/restore"
	"github.com/mpyw/suve/internal/cli/commands/secret/show"
	"github.com/mpyw/suve/internal/cli/commands/secret/update"
)

// Command returns the secret command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "secret",
		Aliases: []string{"sm"},
		Usage:   "Interact with AWS Secrets Manager",
		Commands: []*cli.Command{
			show.Command(),
			log.Command(),
			diff.Command(),
			list.Command(),
			create.Command(),
			update.Command(),
			secretdelete.Command(),
			restore.Command(),
			{
				Name:   "set",
				Hidden: true,
				Action: func(_ context.Context, _ *cli.Command) error {
					return fmt.Errorf(`'suve secret set' is not available

Secrets Manager distinguishes between creating and updating secrets:
  suve secret create <name> <value>   Create a new secret
  suve secret update <name> <value>   Update an existing secret

Unlike SSM Parameter Store, these operations use different AWS APIs`)
				},
			},
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
