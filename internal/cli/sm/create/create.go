// Package create provides the SM create command.
package create

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
)

// Client is the interface for the create command.
type Client interface {
	smapi.CreateSecretAPI
}

// Command returns the create command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new secret",
		ArgsUsage: "<name> <value>",
		Description: `Create a new secret in AWS Secrets Manager.

Use this command for new secrets only. To update an existing secret,
use 'suve sm set' instead.

Secret values are automatically encrypted by Secrets Manager using
the default KMS key or a custom KMS key configured in the account.

EXAMPLES:
   suve sm create my-api-key "sk-12345"                    Create simple secret
   suve sm create -d "API Key for service X" my-key "..."  With description
   suve sm create my-config '{"host":"db.example.com"}'    Create JSON secret`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "description",
				Aliases: []string{"d"},
				Usage:   "Description for the secret",
			},
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve sm create <name> <value>")
	}

	name := c.Args().Get(0)
	value := c.Args().Get(1)
	description := c.String("description")

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name, value, description)
}

// Run executes the create command.
func Run(ctx context.Context, client Client, w io.Writer, name, value, description string) error {
	input := &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String(value),
	}
	if description != "" {
		input.Description = aws.String(description)
	}

	result, err := client.CreateSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(w, "%s Created secret %s (version: %s)\n",
		green("✓"),
		aws.ToString(result.Name),
		aws.ToString(result.VersionId),
	)

	return nil
}
