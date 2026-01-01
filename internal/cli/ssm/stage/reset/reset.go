// Package reset provides the SSM reset command for unstaging or restoring parameters.
package reset

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/cli/ssm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stage/stagerunner"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Command returns the reset command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "reset",
		Usage:     "Unstage parameter or restore to specific version",
		ArgsUsage: "[spec]",
		Description: `Remove a parameter from staging area or restore to a specific version.

Without a version specifier, the parameter is simply removed from staging.
With a version specifier, the value at that version is fetched and staged.

Use 'suve ssm stage reset --all' to unstage all SSM parameters at once.

VERSION SPECIFIERS:
   /app/config          Unstage parameter (remove from staging)
   /app/config#3        Restore to version 3
   /app/config~1        Restore to 1 version ago

EXAMPLES:
   suve ssm stage reset /app/config              Unstage (remove from staging)
   suve ssm stage reset /app/config#3            Stage value from version 3
   suve ssm stage reset /app/config~1            Stage value from previous version
   suve ssm stage reset --all                    Unstage all SSM parameters`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Unstage all SSM parameters",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	resetAll := cmd.Bool("all")

	if !resetAll && cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm stage reset <spec> or suve ssm stage reset --all")
	}

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	opts := stagerunner.ResetOptions{
		All: resetAll,
	}
	if !resetAll {
		opts.Spec = cmd.Args().First()
	}

	// Check if version spec is provided (need AWS client)
	needsAWS := false
	if !resetAll && opts.Spec != "" {
		spec, err := ssmversion.Parse(opts.Spec)
		if err != nil {
			return err
		}
		needsAWS = spec.Absolute.Version != nil || spec.Shift > 0
	}

	var strat *strategy.Strategy
	if needsAWS {
		client, err := awsutil.NewSSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize AWS client: %w", err)
		}
		strat = strategy.NewStrategy(client)
	} else {
		strat = strategy.NewStrategy(nil)
	}

	r := &stagerunner.ResetRunner{
		Strategy: strat,
		Store:    store,
		Stdout:   cmd.Root().Writer,
		Stderr:   cmd.Root().ErrWriter,
	}

	return r.Run(ctx, opts)
}
