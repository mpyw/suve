// Package stage provides the global stage command for managing staged changes.
package stage

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/stage/agent"
	"github.com/mpyw/suve/internal/cli/commands/stage/apply"
	"github.com/mpyw/suve/internal/cli/commands/stage/diff"
	"github.com/mpyw/suve/internal/cli/commands/stage/drain"
	"github.com/mpyw/suve/internal/cli/commands/stage/param"
	"github.com/mpyw/suve/internal/cli/commands/stage/persist"
	"github.com/mpyw/suve/internal/cli/commands/stage/reset"
	"github.com/mpyw/suve/internal/cli/commands/stage/secret"
	"github.com/mpyw/suve/internal/cli/commands/stage/status"
)

// Command returns the global stage command with subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "stage",
		Usage: "Manage staged changes for AWS Parameter Store and Secrets Manager",
		Description: `Stage changes locally before applying to AWS.

Use 'suve stage param' for SSM Parameter Store operations.
Use 'suve stage secret' for Secrets Manager operations.

Global commands operate on all staged changes:
   status    Show all staged changes (SSM Parameter Store and Secrets Manager)
   diff      Show diff of all staged changes vs AWS
   apply     Apply all staged changes to AWS
   reset     Unstage all changes

EXAMPLES:
   suve stage param add /my/param       Stage a new SSM Parameter Store parameter
   suve stage secret edit my-secret     Edit and stage a secret
   suve stage status                    View all staged changes
   suve stage apply                     Apply all staged changes`,
		Commands: []*cli.Command{
			param.Command(),
			secret.Command(),
			status.Command(),
			diff.Command(),
			apply.Command(),
			reset.Command(),
			persist.Command(),
			drain.Command(),
			agent.Command(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
