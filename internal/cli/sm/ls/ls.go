// Package ls provides the SM ls command.
package ls

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the ls command.
type Client interface {
	smapi.ListSecretsAPI
}

// Runner executes the ls command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the ls command.
type Options struct {
	Prefix string
}

// Command returns the ls command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "ls",
		Usage:     "List secrets",
		ArgsUsage: "[filter-prefix]",
		Description: `List secrets in AWS Secrets Manager.

Without a filter prefix, lists all secrets in the account.
With a filter prefix, lists only secrets whose names contain that prefix.

Note: Unlike SSM parameters, Secrets Manager filters by name substring,
not by path hierarchy.

EXAMPLES:
   suve sm ls                  List all secrets
   suve sm ls prod             List secrets containing "prod"
   suve sm ls my-app/          List secrets starting with "my-app/"`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Prefix: cmd.Args().First(),
	})
}

// Run executes the ls command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	input := &secretsmanager.ListSecretsInput{}
	if opts.Prefix != "" {
		input.Filters = []types.Filter{
			{
				Key:    types.FilterNameStringTypeName,
				Values: []string{opts.Prefix},
			},
		}
	}

	paginator := secretsmanager.NewListSecretsPaginator(r.Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list secrets: %w", err)
		}

		for _, secret := range page.SecretList {
			_, _ = fmt.Fprintln(r.Stdout, lo.FromPtr(secret.Name))
		}
	}

	return nil
}
