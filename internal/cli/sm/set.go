package sm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	internalaws "github.com/mpyw/suve/internal/aws"
)

func setCommand() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Update a secret value",
		ArgsUsage: "<name> <value>",
		Action: func(c *cli.Context) error {
			if c.NArg() < 2 {
				return fmt.Errorf("usage: suve sm set <name> <value>")
			}
			ctx := c.Context
			cfg, err := internalaws.LoadConfig(ctx)
			if err != nil {
				return err
			}
			client := secretsmanager.NewFromConfig(cfg)
			name := c.Args().Get(0)
			value := c.Args().Get(1)
			return runSet(ctx, c.App.Writer, client, name, value)
		},
	}
}

func runSet(ctx context.Context, w io.Writer, client SetClient, name, value string) error {
	result, err := client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(name),
		SecretString: aws.String(value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(w, "%s Updated secret %s (version: %s)\n",
		green("✓"),
		aws.ToString(result.Name),
		aws.ToString(result.VersionId),
	)

	return nil
}
