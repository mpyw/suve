// Package status provides the SM status command for viewing staged changes.
package status

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/sm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stage/stagerunner"
)

// Command returns the status command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Show staged secret changes",
		ArgsUsage: "[name]",
		Description: `Display staged changes for AWS Secrets Manager.

Without arguments, shows all staged secret changes.
With a secret name, shows the staged change for that specific secret.

Use -v/--verbose to show detailed information including the staged value.

EXAMPLES:
   suve sm stage status             Show all staged SM changes
   suve sm stage status my-secret   Show staged change for specific secret
   suve sm stage status -v          Show detailed information`,
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
