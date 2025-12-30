// Package ls provides the SM ls command.
package ls

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the ls command.
type Client interface {
	smapi.ListSecretsAPI
}

// Command returns the ls command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "ls",
		Usage:     "List secrets",
		ArgsUsage: "[filter-prefix]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "filter",
				Usage: "Filter secrets by name prefix",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	prefix := c.Args().First()
	if prefix == "" {
		prefix = c.String("filter")
	}

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, prefix)
}

// Run executes the ls command.
func Run(ctx context.Context, client Client, w io.Writer, prefix string) error {
	input := &secretsmanager.ListSecretsInput{}
	if prefix != "" {
		input.Filters = []types.Filter{
			{
				Key:    types.FilterNameStringTypeName,
				Values: []string{prefix},
			},
		}
	}

	paginator := secretsmanager.NewListSecretsPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list secrets: %w", err)
		}

		for _, secret := range page.SecretList {
			_, _ = fmt.Fprintln(w, aws.ToString(secret.Name))
		}
	}

	return nil
}
