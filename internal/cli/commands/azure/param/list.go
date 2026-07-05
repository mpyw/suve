package param

import (
	"context"

	"github.com/urfave/cli/v3"

	genericlist "github.com/mpyw/suve/internal/cli/commands/generic/list"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/usecase/azure"
)

// ListCommand returns the Azure App Configuration list command.
func ListCommand() *cli.Command {
	return genericlist.Command(genericlist.Config{
		Usage:     "List settings",
		ArgsUsage: "[filter-prefix]",
		Description: `List settings (key-values) in Azure App Configuration.

Without a filter prefix, lists all keys in the store.
With a filter prefix, lists only keys that start with that prefix.

FILTERING:
   Use --filter to filter results by regex pattern (client-side).

VALUE DISPLAY:
   Use --show to display setting values alongside keys.
   Output format: <key><TAB><value>

EXAMPLES:
   suve azure param list                     List all settings
   suve azure param list app/                List settings starting with "app/"
   suve azure param list --show app/         List with values
   suve azure param list --output=json app/  List as JSON`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "filter",
				Usage: "Filter by regex pattern",
			},
			&cli.BoolFlag{
				Name:  "show",
				Usage: "Show setting values",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		NewList: func(
			ctx context.Context, cmd *cli.Command, withValue bool,
		) (func(context.Context) ([]genericlist.Entry, error), error) {
			store, err := cliinternal.AzureAppConfigStore(ctx)
			if err != nil {
				return nil, err
			}

			uc := &azure.ListUseCase{Reader: store}
			input := azure.ListInput{
				Prefix:    cmd.Args().First(),
				Filter:    cmd.String("filter"),
				WithValue: withValue,
			}

			return func(ctx context.Context) ([]genericlist.Entry, error) {
				result, err := uc.Execute(ctx, input)
				if err != nil {
					return nil, err
				}

				entries := make([]genericlist.Entry, len(result.Entries))
				for i, e := range result.Entries {
					entries[i] = genericlist.Entry{Name: e.Name, Value: e.Value, Error: e.Error}
				}

				return entries, nil
			}, nil
		},
	})
}
