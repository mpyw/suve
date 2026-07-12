package param

import (
	"context"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	genericlist "github.com/mpyw/suve/internal/cli/commands/generic/list"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/usecase/param"
)

// ListCommand returns the SSM Parameter Store list command.
func ListCommand() *cli.Command {
	return genericlist.Command(genericlist.Config{
		Usage:     "List parameters",
		ArgsUsage: "[path-prefix]",
		Description: `List parameters in AWS Systems Manager Parameter Store.

Without a path prefix, lists all parameters in the account.
With a path prefix, lists only parameters under that path.

By default, lists only immediate children of the path.
Use --recursive to include all descendant parameters.

FILTERING:
   Use --filter to filter results by regex pattern (client-side).
   The pattern is matched against the full parameter name.

VALUE DISPLAY:
   Use --show to display parameter values alongside names.
   Output format: <name><TAB><value>

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
   suve param list                          List all parameters
   suve param list /app                     List parameters directly under /app
   suve param list --recursive /app         List all parameters under /app recursively
   suve param list /app/config/             List parameters under /app/config
   suve param list --filter '\.prod\.'      List parameters matching regex
   suve param list --show /app              List with values
   suve param list --output=json /app       List as JSON`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "recursive",
				Aliases: []string{"R"},
				Usage:   "List recursively",
			},
			&cli.StringFlag{
				Name:  "filter",
				Usage: "Filter by regex pattern",
			},
			&cli.BoolFlag{
				Name:  "show",
				Usage: "Show parameter values",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		NewList: func(
			ctx context.Context, cmd *cli.Command, withValue bool,
		) (func(context.Context) ([]genericlist.Entry, error), error) {
			store, err := cliinternal.ParamStore(ctx)
			if err != nil {
				return nil, err
			}

			uc := &param.ListUseCase{Reader: store}
			input := param.ListInput{
				Prefix:    cmd.Args().First(),
				Recursive: cmd.Bool("recursive"),
				Filter:    cmd.String("filter"),
				WithValue: withValue,
			}

			return func(ctx context.Context) ([]genericlist.Entry, error) {
				result, err := uc.Execute(ctx, input)
				if err != nil {
					return nil, err
				}

				entries := lo.Map(result.Entries, func(e param.ListEntry, _ int) genericlist.Entry {
					return genericlist.Entry{Name: e.Name, Value: e.Value, Error: e.Error}
				})

				return entries, nil
			}, nil
		},
	})
}
