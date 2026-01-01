// Package stage provides the SM stage subcommand for staging operations.
package stage

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/sm/stage/delete"
	"github.com/mpyw/suve/internal/cli/sm/stage/diff"
	"github.com/mpyw/suve/internal/cli/sm/stage/edit"
	"github.com/mpyw/suve/internal/cli/sm/stage/push"
	"github.com/mpyw/suve/internal/cli/sm/stage/reset"
	"github.com/mpyw/suve/internal/cli/sm/stage/status"
)

// Command returns the stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "stage",
		Usage: "Staging operations for Secrets Manager",
		Description: `Stage changes locally before pushing to AWS.

Use 'suve sm stage edit' to edit and stage a secret.
Use 'suve sm stage delete' to stage a secret for deletion.
Use 'suve sm stage status' to view staged changes.
Use 'suve sm stage diff' to see differences between staged and AWS values.
Use 'suve sm stage push' to apply staged changes to AWS.
Use 'suve sm stage reset' to unstage or restore from a version.`,
		Commands: []*cli.Command{
			edit.Command(),
			delete.Command(),
			status.Command(),
			diff.Command(),
			push.Command(),
			reset.Command(),
		},
	}
}
