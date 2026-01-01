// Package status provides the SSM status command for viewing staged changes.
package status

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/ssm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stage/stagerunner"
)

// Command returns the status command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Show staged parameter changes",
		ArgsUsage: "[name]",
		Description: `Display staged changes for SSM Parameter Store.

Without arguments, shows all staged SSM parameter changes.
With a parameter name, shows the staged change for that specific parameter.

Use -v/--verbose to show detailed information including the staged value.

EXAMPLES:
   suve ssm stage status              Show all staged SSM changes
   suve ssm stage status /app/config  Show staged change for specific parameter
   suve ssm stage status -v           Show detailed information`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show detailed information including values",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	r := &stagerunner.StatusRunner{
		Strategy: strategy.NewStrategy(nil),
		Store:    store,
		Stdout:   cmd.Root().Writer,
		Stderr:   cmd.Root().ErrWriter,
	}

	opts := stagerunner.StatusOptions{
		Verbose: cmd.Bool("verbose"),
	}
	if cmd.Args().Len() > 0 {
		opts.Name = cmd.Args().First()
	}

	return r.Run(ctx, opts)
}
