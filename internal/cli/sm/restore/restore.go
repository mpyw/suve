// Package restore provides the SM restore command.
package restore

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

// Client is the interface for the restore command.
type Client interface {
	RestoreSecret(ctx context.Context, params *secretsmanager.RestoreSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error)
}

// Command returns the restore command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "restore",
		Usage:     "Restore a deleted secret",
		ArgsUsage: "<name>",
		Action:    action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("usage: suve sm restore <name>")
	}

	name := c.Args().First()

	client, err := internalaws.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name)
}

// Run executes the restore command.
func Run(ctx context.Context, client Client, w io.Writer, name string) error {
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
