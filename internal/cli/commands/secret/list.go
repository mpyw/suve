package secret

import (
	"context"

	"github.com/urfave/cli/v3"

	genericlist "github.com/mpyw/suve/internal/cli/commands/generic/list"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	awssecret "github.com/mpyw/suve/internal/provider/aws/secret"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// ListCommand returns the Secrets Manager list command.
func ListCommand() *cli.Command {
	return genericlist.Command(genericlist.Config{
		Usage:     "List secrets",
		ArgsUsage: "[filter-prefix]",
		Description: `List secrets in AWS Secrets Manager.

Without a filter prefix, lists all secrets in the account.
With a filter prefix, lists only secrets whose names contain that prefix.

Note: Unlike SSM parameters, Secrets Manager filters by name substring,
not by path hierarchy.

FILTERING:
   Use --filter to filter results by regex pattern (client-side).
   The pattern is matched against the full secret name.

VALUE DISPLAY:
   Use --show to display secret values alongside names.
   Output format: <name><TAB><value>

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
   suve secret list                       List all secrets
   suve secret list prod                  List secrets containing "prod"
   suve secret list my-app/               List secrets starting with "my-app/"
   suve secret list --filter '\.prod$'    List secrets matching regex
   suve secret list --show prod           List with values
   suve secret list --output=json prod    List as JSON`,
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
			client, err := cliinternal.NewSecretClient(ctx)
			if err != nil {
				return nil, err
			}

			uc := &secret.ListUseCase{Reader: awssecret.New(client)}
			input := secret.ListInput{
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
