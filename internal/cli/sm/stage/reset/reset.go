// Package reset provides the SM reset command for unstaging or restoring secrets.
package reset

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/cli/sm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stageutil"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Command returns the reset command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "reset",
		Usage:     "Unstage secret or restore to specific version",
		ArgsUsage: "[spec]",
		Description: `Remove a secret from staging area or restore to a specific version.

Without a version specifier, the secret is simply removed from staging.
With a version specifier, the value at that version is fetched and staged.

Use 'suve sm stage reset --all' to unstage all SM secrets at once.

VERSION SPECIFIERS:
   my-secret            Unstage secret (remove from staging)
   my-secret#abc123     Restore to specific version ID
   my-secret:AWSPREVIOUS  Restore to AWSPREVIOUS label
   my-secret~1          Restore to 1 version ago

EXAMPLES:
   suve sm stage reset my-secret              Unstage (remove from staging)
   suve sm stage reset my-secret#abc123       Stage value from specific version
   suve sm stage reset my-secret:AWSPREVIOUS  Stage value from AWSPREVIOUS
   suve sm stage reset my-secret~1            Stage value from previous version
   suve sm stage reset --all                  Unstage all SM secrets`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Unstage all SM secrets",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	resetAll := cmd.Bool("all")

	if !resetAll && cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm stage reset <spec> or suve sm stage reset --all")
	}

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	opts := stageutil.ResetOptions{
		All: resetAll,
	}
	if !resetAll {
		opts.Spec = cmd.Args().First()
	}

	// Check if version spec is provided (need AWS client)
	needsAWS := false
	if !resetAll && opts.Spec != "" {
		spec, err := smversion.Parse(opts.Spec)
		if err != nil {
			return err
		}
		needsAWS = spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0
	}

	var strat *strategy.Strategy
	if needsAWS {
		client, err := awsutil.NewSMClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize AWS client: %w", err)
		}
		strat = strategy.NewStrategy(client)
	} else {
		strat = strategy.NewStrategy(nil)
	}

	r := &stageutil.ResetRunner{
		Strategy: strat,
		Store:    store,
		Stdout:   cmd.Root().Writer,
		Stderr:   cmd.Root().ErrWriter,
	}

	return r.Run(ctx, opts)
}
