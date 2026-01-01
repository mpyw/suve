// Package edit provides the SSM edit command for staging parameter changes.
package edit

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/cli/ssm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stage/stagerunner"
)

// Command returns the edit command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "edit",
		Usage:     "Edit parameter value and stage changes",
		ArgsUsage: "<name>",
		Description: `Open an editor to modify a parameter value, then stage the change.

If the parameter is already staged, edits the staged value.
Otherwise, fetches the current value from AWS and opens it for editing.
Saves the edited value to the staging area (does not immediately push to AWS).

Use 'suve ssm stage delete' to stage a parameter for deletion.
Use 'suve ssm stage push' to apply staged changes to AWS.
Use 'suve ssm stage status' to view staged changes.

EXAMPLES:
   suve ssm stage edit /app/config/db-url  Edit and stage parameter`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm stage edit <name>")
	}

	name := cmd.Args().First()

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &stagerunner.EditRunner{
		Strategy: strategy.NewStrategy(client),
		Store:    store,
		Stdout:   cmd.Root().Writer,
		Stderr:   cmd.Root().ErrWriter,
	}
	return r.Run(ctx, stagerunner.EditOptions{Name: name})
}
