// Package push provides the SM push command for applying staged changes.
package push

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/cli/sm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stageutil"
)

// Command returns the push command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "push",
		Usage:     "Apply staged secret changes to AWS",
		ArgsUsage: "[name]",
		Description: `Apply all staged Secrets Manager changes to AWS.

If a secret name is specified, only that secret's staged changes are applied.
Otherwise, all staged SM secret changes are applied.

After successful push, the staged changes are cleared.

Use 'suve sm stage status' to view staged changes before pushing.

EXAMPLES:
   suve sm stage push              Push all staged SM changes
   suve sm stage push my-secret    Push only the specified secret`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(client),
		Store:    store,
		Stdout:   cmd.Root().Writer,
		Stderr:   cmd.Root().ErrWriter,
	}

	opts := stageutil.PushOptions{}
	if cmd.Args().Len() > 0 {
		opts.Name = cmd.Args().First()
	}

	return r.Run(ctx, opts)
}
