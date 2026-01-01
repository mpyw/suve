// Package diff provides the SSM stage diff command for comparing staged vs AWS values.
package diff

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/cli/ssm/strategy"
	"github.com/mpyw/suve/internal/pager"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stage/stagerunner"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between staged and AWS values",
		ArgsUsage: "[name]",
		Description: `Compare staged values against AWS current values.

If a parameter name is specified, shows diff for that parameter only.
Otherwise, shows diff for all staged SSM parameters.

EXAMPLES:
   suve ssm stage diff              Show diff for all staged SSM parameters
   suve ssm stage diff /app/config  Show diff for specific parameter
   suve ssm stage diff -j           Show diff with JSON formatting`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	var name string
	if cmd.Args().Len() > 1 {
		return fmt.Errorf("usage: suve ssm stage diff [name]")
	}
	if cmd.Args().Len() == 1 {
		// Parse and validate the name (no version specifier allowed)
		spec, err := ssmversion.Parse(cmd.Args().First())
		if err != nil {
			return err
		}
		if spec.Absolute.Version != nil || spec.Shift > 0 {
			return fmt.Errorf("stage diff requires a parameter name without version specifier")
		}
		name = spec.Name
	}

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	opts := stagerunner.DiffOptions{
		Name:       name,
		JSONFormat: cmd.Bool("json"),
		NoPager:    cmd.Bool("no-pager"),
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
		r := &stagerunner.DiffRunner{
			Strategy: strategy.NewStrategy(client),
			Store:    store,
			Stdout:   w,
			Stderr:   cmd.Root().ErrWriter,
		}
		return r.Run(ctx, opts)
	})
}
