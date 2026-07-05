package gcloud

import (
	"context"

	"github.com/urfave/cli/v3"

	genericlist "github.com/mpyw/suve/internal/cli/commands/generic/list"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/usecase/gcloud"
)

// ListCommand returns the Google Cloud Secret Manager list command.
func ListCommand() *cli.Command {
	return genericlist.Command(genericlist.Config{
		Usage:     "List secrets",
		ArgsUsage: "[filter-prefix]",
		Description: `List secrets in Google Cloud Secret Manager.

Without a filter prefix, lists all secrets in the project.
With a filter prefix, lists only secrets whose names start with that prefix.

FILTERING:
   Use --filter to filter results by regex pattern (client-side).

VALUE DISPLAY:
   Use --show to display secret values alongside names.
   Output format: <name><TAB><value>

EXAMPLES:
   suve gcloud secret list                     List all secrets
   suve gcloud secret list prod                List secrets starting with "prod"
   suve gcloud secret list --show prod         List with values
   suve gcloud secret list --output=json prod  List as JSON`,
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
		) (func(context.Context) ([]genericlist.Entry, error), error) {
			store, err := cliinternal.GoogleCloudSecretStore(ctx)
			if err != nil {
				return nil, err
			}

			uc := &gcloud.ListUseCase{Reader: store}
			input := gcloud.ListInput{
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
