// Package stage provides the global stage command for managing staged changes.
package stage

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/stage/diff"
	"github.com/mpyw/suve/internal/cli/commands/stage/push"
	"github.com/mpyw/suve/internal/cli/commands/stage/reset"
	"github.com/mpyw/suve/internal/cli/commands/stage/sm"
	"github.com/mpyw/suve/internal/cli/commands/stage/ssm"
	"github.com/mpyw/suve/internal/cli/commands/stage/status"
)

// Command returns the global stage command with subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "stage",
		Usage: "Manage staged changes for AWS Parameter Store and Secrets Manager",
		Description: `Stage changes locally before pushing to AWS.

Use 'suve stage ssm' for SSM Parameter Store operations.
Use 'suve stage sm' for Secrets Manager operations.

Global commands operate on all staged changes:
   status    Show all staged changes (SSM and SM)
   diff      Show diff of all staged changes vs AWS
   push      Apply all staged changes to AWS
   reset     Unstage all changes

EXAMPLES:
   suve stage ssm add /my/param         Stage a new SSM parameter
   suve stage sm edit my-secret         Edit and stage a secret
   suve stage status                    View all staged changes
   suve stage push                      Apply all staged changes`,
		Commands: []*cli.Command{
			ssm.Command(),
			sm.Command(),
			status.Command(),
			diff.Command(),
			push.Command(),
			reset.Command(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
