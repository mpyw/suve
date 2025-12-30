// Package set provides the SM set command.
package set

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

// Client is the interface for the set command.
type Client interface {
	smapi.PutSecretValueAPI
}

// Command returns the set command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Update a secret value",
		ArgsUsage: "<name> <value>",
		Description: `Update the value of an existing secret.

This creates a new version of the secret. The previous version will
have its AWSCURRENT label moved to AWSPREVIOUS.

Use 'suve sm create' to create a new secret.

EXAMPLES:
   suve sm set my-api-key "new-key-value"            Update with new value
   suve sm set my-config '{"host":"new-db.com"}'     Update JSON secret`,
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve sm set <name> <value>")
	}

	name := c.Args().Get(0)
	value := c.Args().Get(1)

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name, value)
}

// Run executes the set command.
func Run(ctx context.Context, client Client, w io.Writer, name, value string) error {
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
