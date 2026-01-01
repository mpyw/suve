// Package create provides the SM create command.
package create

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/tagging"
)

// Client is the interface for the create command.
type Client interface {
	smapi.CreateSecretAPI
}

// Runner executes the create command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the create command.
type Options struct {
	Name        string
	Value       string
	Description string
	Tags        map[string]string
}

// Command returns the create command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new secret",
		ArgsUsage: "<name> <value>",
		Description: `Create a new secret in AWS Secrets Manager.

Use this command for new secrets only. To update an existing secret,
use 'suve sm update' instead.

Secret values are automatically encrypted by Secrets Manager using
the default KMS key or a custom KMS key configured in the account.

EXAMPLES:
   suve sm create my-api-key "sk-12345"                    Create simple secret
   suve sm create --description "API Key for service X" my-key "..."  With description
   suve sm create my-config '{"host":"db.example.com"}'    Create JSON secret`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: "Description for the secret",
			},
			&cli.StringSliceFlag{
				Name:  "tag",
				Usage: "Tag in key=value format (can be specified multiple times)",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve sm create <name> <value>")
	}

	// Parse tags (untag doesn't make sense for create)
	tagResult, err := tagging.ParseFlags(cmd.StringSlice("tag"), nil)
	if err != nil {
		return err
	}

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
		Name:        cmd.Args().Get(0),
		Value:       cmd.Args().Get(1),
		Description: cmd.String("description"),
		Tags:        tagResult.Change.Add,
	})
}

// Run executes the create command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	input := &secretsmanager.CreateSecretInput{
		Name:         lo.ToPtr(opts.Name),
		SecretString: lo.ToPtr(opts.Value),
	}
	if opts.Description != "" {
		input.Description = lo.ToPtr(opts.Description)
	}
	if len(opts.Tags) > 0 {
		input.Tags = make([]types.Tag, 0, len(opts.Tags))
		for k, v := range opts.Tags {
			input.Tags = append(input.Tags, types.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
	}

	result, err := r.Client.CreateSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Created secret %s (version: %s)\n",
		green("✓"),
		lo.FromPtr(result.Name),
		lo.FromPtr(result.VersionId),
	)

	return nil
}
