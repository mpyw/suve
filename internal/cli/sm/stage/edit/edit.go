// Package edit provides the SM edit command for staging secret changes.
package edit

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/cli/sm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stage/stagerunner"
)

// Command returns the edit command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "edit",
		Usage:     "Edit secret value and stage changes",
		ArgsUsage: "<name>",
		Description: `Open an editor to modify a secret value, then stage the change.

If the secret is already staged, edits the staged value.
Otherwise, fetches the current value from AWS and opens it for editing.
Saves the edited value to the staging area (does not immediately push to AWS).

Use 'suve sm stage delete' to stage a secret for deletion.
Use 'suve sm stage push' to apply staged changes to AWS.
Use 'suve sm stage status' to view staged changes.

EXAMPLES:
   suve sm stage edit my-secret  Edit and stage secret`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm stage edit <name>")
	}

	name := cmd.Args().First()

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSMClient(ctx)
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
