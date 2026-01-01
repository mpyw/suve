package sm

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/sm/cat"
	"github.com/mpyw/suve/internal/cli/sm/create"
	smdelete "github.com/mpyw/suve/internal/cli/sm/delete"
	"github.com/mpyw/suve/internal/cli/sm/diff"
	"github.com/mpyw/suve/internal/cli/sm/log"
	"github.com/mpyw/suve/internal/cli/sm/ls"
	"github.com/mpyw/suve/internal/cli/sm/restore"
	"github.com/mpyw/suve/internal/cli/sm/show"
	"github.com/mpyw/suve/internal/cli/sm/stage"
	"github.com/mpyw/suve/internal/cli/sm/update"
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
			stage.Command(),
			setDeprecatedCommand(),
		},
	}
}

// setDeprecatedCommand returns a hidden command that explains why 'set' is not available.
func setDeprecatedCommand() *cli.Command {
	return &cli.Command{
		Name:   "set",
		Hidden: true,
		Action: func(_ context.Context, _ *cli.Command) error {
			return fmt.Errorf(`'suve sm set' is not available

Secrets Manager distinguishes between creating and updating secrets:
  suve sm create <name> <value>   Create a new secret
  suve sm update <name> <value>   Update an existing secret

Unlike SSM Parameter Store, these operations use different AWS APIs`)
		},
	}
}
