// Package push provides the SSM push command for applying staged changes.
package push

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/cli/ssm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stageutil"
)

// Command returns the push command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "push",
		Usage:     "Apply staged parameter changes to AWS",
		ArgsUsage: "[name]",
		Description: `Apply all staged SSM parameter changes to AWS.

If a parameter name is specified, only that parameter's staged changes are applied.
Otherwise, all staged SSM parameter changes are applied.

After successful push, the staged changes are cleared.

Use 'suve ssm stage status' to view staged changes before pushing.

EXAMPLES:
   suve ssm stage push                    Push all staged SSM changes
   suve ssm stage push /app/config/db     Push only the specified parameter`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSSMClient(ctx)
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
