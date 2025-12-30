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

func restoreCommand() *cli.Command {
	return &cli.Command{
		Name:      "restore",
		Usage:     "Restore a deleted secret",
		ArgsUsage: "<name>",
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return fmt.Errorf("usage: suve sm restore <name>")
			}
			ctx := c.Context
			cfg, err := internalaws.LoadConfig(ctx)
			if err != nil {
				return err
			}
			client := secretsmanager.NewFromConfig(cfg)
			name := c.Args().First()
			return runRestore(ctx, c.App.Writer, client, name)
		},
	}
}

func runRestore(ctx context.Context, w io.Writer, client RestoreClient, name string) error {
	result, err := client.RestoreSecret(ctx, &secretsmanager.RestoreSecretInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("failed to restore secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(w, "%s Restored secret %s\n",
		green("✓"),
		aws.ToString(result.Name),
	)

	return nil
}
