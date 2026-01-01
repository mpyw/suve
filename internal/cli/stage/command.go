// Package stage provides the global stage command for managing staged changes.
package stage

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/stage/diff"
	"github.com/mpyw/suve/internal/cli/stage/push"
	"github.com/mpyw/suve/internal/cli/stage/reset"
	"github.com/mpyw/suve/internal/cli/stage/status"
)

// Command returns the global stage command with subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "stage",
		Usage: "Manage staged changes (SSM and SM)",
		Description: `Manage staged changes for both SSM Parameter Store and Secrets Manager.

Staging allows you to prepare changes locally before pushing to AWS.
Use service-specific stage commands (suve ssm stage, suve sm stage)
for staging individual parameters or secrets.

SUBCOMMANDS:
   status    Show all staged changes
   diff      Show diff of staged changes vs AWS
   push      Apply all staged changes to AWS
   reset     Unstage all changes

EXAMPLES:
   suve stage status    View all staged changes
   suve stage diff      Compare staged values with AWS
   suve stage push      Apply all staged changes
   suve stage reset     Clear all staged changes`,
		Commands: []*cli.Command{
			status.Command(),
			diff.Command(),
			push.Command(),
			reset.Command(),
		},
	}
}
