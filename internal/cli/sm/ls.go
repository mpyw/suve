package sm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/urfave/cli/v2"

	internalaws "github.com/mpyw/suve/internal/aws"
)

func lsCommand() *cli.Command {
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
		Action: func(c *cli.Context) error {
			ctx := c.Context
			cfg, err := internalaws.LoadConfig(ctx)
			if err != nil {
				return err
			}
			client := secretsmanager.NewFromConfig(cfg)
			prefix := c.Args().First()
			if prefix == "" {
				prefix = c.String("filter")
			}
			return runLs(ctx, c.App.Writer, client, prefix)
		},
	}
}

func runLs(ctx context.Context, w io.Writer, client LsClient, prefix string) error {
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
