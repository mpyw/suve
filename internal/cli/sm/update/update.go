// Package update provides the SM update command.
package update

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

// Client is the interface for the update command.
type Client interface {
	smapi.PutSecretValueAPI
}

// Runner executes the update command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the update command.
type Options struct {
	Name  string
	Value string
}

// Command returns the update command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a secret value",
		ArgsUsage: "<name> <value>",
		Description: `Update the value of an existing secret.

This creates a new version of the secret. The previous version will
have its AWSCURRENT label moved to AWSPREVIOUS.

Use 'suve sm create' to create a new secret.

EXAMPLES:
  suve sm update my-api-key "new-key-value"            Update with new value
  suve sm update my-config '{"host":"new-db.com"}'     Update JSON secret`,
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve sm update <name> <value>")
	}

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: c.App.Writer,
		Stderr: c.App.ErrWriter,
	}
	return r.Run(c.Context, Options{
		Name:  c.Args().Get(0),
		Value: c.Args().Get(1),
	})
}

// Run executes the update command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.Client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(opts.Name),
		SecretString: aws.String(opts.Value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Updated secret %s (version: %s)\n",
		green("✓"),
		aws.ToString(result.Name),
		aws.ToString(result.VersionId),
	)

	return nil
}
