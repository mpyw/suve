package secret

import (
	"context"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// ListCommand returns the Azure Key Vault list command.
func ListCommand() *cli.Command {
	return generic.ListCommand(generic.ListConfig{
		Usage:     "List secrets",
		ArgsUsage: "[filter-prefix]",
		Description: `List secrets in Azure Key Vault.

Without a filter prefix, lists all secrets in the vault.
With a filter prefix, lists only secrets whose names start with that prefix.

FILTERING:
   Use --filter to filter results by regex pattern (client-side).

VALUE DISPLAY:
   Use --show to display secret values alongside names.
   Output format: <name><TAB><value>

EXAMPLES:
   suve azure secret list                     List all secrets
   suve azure secret list prod                List secrets starting with "prod"
   suve azure secret list --show prod         List with values
   suve azure secret list --output=json prod  List as JSON`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "filter",
				Usage: "Filter by regex pattern",
			},
			&cli.BoolFlag{
				Name:  "show",
				Usage: "Show secret values",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		NewList: func(
			ctx context.Context, cmd *cli.Command, withValue bool,
		) (func(context.Context) ([]generic.ListEntry, error), error) {
			store, err := azureinternal.KeyVaultStore(ctx)
			if err != nil {
				return nil, err
			}

			uc := &secret.ListUseCase{Reader: store}
			input := secret.ListInput{
				Prefix:    cmd.Args().First(),
				Filter:    cmd.String("filter"),
				WithValue: withValue,
			}

			return func(ctx context.Context) ([]generic.ListEntry, error) {
				result, err := uc.Execute(ctx, input)
				if err != nil {
					return nil, err
				}

				entries := lo.Map(result.Entries, func(e secret.ListEntry, _ int) generic.ListEntry {
					return generic.ListEntry{Name: e.Name, Value: e.Value, Error: e.Error}
				})

				return entries, nil
			}, nil
		},
	})
}
