// Package diff provides the SM stage diff command for comparing staged vs AWS values.
package diff

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/cli/sm/strategy"
	"github.com/mpyw/suve/internal/pager"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stage/stagerunner"
	"github.com/mpyw/suve/internal/version/smversion"
)

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between staged and AWS values",
		ArgsUsage: "[name]",
		Description: `Compare staged values against AWS current values.

If a secret name is specified, shows diff for that secret only.
Otherwise, shows diff for all staged SM secrets.

EXAMPLES:
   suve sm stage diff            Show diff for all staged SM secrets
   suve sm stage diff my-secret  Show diff for specific secret
   suve sm stage diff -j         Show diff with JSON formatting`,
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
		return fmt.Errorf("usage: suve sm stage diff [name]")
	}
	if cmd.Args().Len() == 1 {
		// Parse and validate the name (no version specifier allowed)
		spec, err := smversion.Parse(cmd.Args().First())
		if err != nil {
			return err
		}
		if spec.Absolute.ID != nil || spec.Absolute.Label != nil || spec.Shift > 0 {
			return fmt.Errorf("stage diff requires a secret name without version specifier")
		}
		name = spec.Name
	}

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	client, err := awsutil.NewSMClient(ctx)
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
