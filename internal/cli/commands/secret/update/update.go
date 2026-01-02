// Package update provides the Secrets Manager update command.
package update

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/tagging"
)

// Client is the interface for the update command.
type Client interface {
	secretapi.PutSecretValueAPI
	secretapi.UpdateSecretAPI
	secretapi.TagResourceAPI
	secretapi.UntagResourceAPI
}

// Runner executes the update command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the update command.
type Options struct {
	Name        string
	Value       string
	Description string
	TagChange   *tagging.Change
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

Use 'suve secret create' to create a new secret.

TAGGING:
   --tag adds or updates tags (additive, does not remove existing tags)
   --untag removes specific tags by key
   If the same key appears in both, the later flag wins with a warning.

EXAMPLES:
  suve secret update my-api-key "new-key-value"             Update with new value
  suve secret update my-config '{"host":"new-db.com"}'      Update JSON secret
  suve secret update --yes my-api-key "new-key-value"       Update without confirmation
  suve secret update --tag env=prod my-api-key "value"      Update with tags
  suve secret update --untag deprecated my-api-key "value"  Remove a tag`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: "Update secret description",
			},
			&cli.StringSliceFlag{
				Name:  "tag",
				Usage: "Tag in key=value format (can be specified multiple times, additive)",
			},
			&cli.StringSliceFlag{
				Name:  "untag",
				Usage: "Tag key to remove (can be specified multiple times)",
			},
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve secret update <name> <value>")
	}

	name := cmd.Args().Get(0)
	skipConfirm := cmd.Bool("yes")

	// Parse tags
	tagResult, err := tagging.ParseFlags(cmd.StringSlice("tag"), cmd.StringSlice("untag"))
	if err != nil {
		return err
	}

	// Output warnings
	for _, w := range tagResult.Warnings {
		output.Warning(cmd.Root().ErrWriter, "%s", w)
	}

	// Confirm operation
	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	confirmed, err := prompter.ConfirmAction("Update secret", name, skipConfirm)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name:        name,
		Value:       cmd.Args().Get(1),
		Description: cmd.String("description"),
		TagChange:   tagResult.Change,
	})
}

// Run executes the update command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	// Update secret value
	result, err := r.Client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     lo.ToPtr(opts.Name),
		SecretString: lo.ToPtr(opts.Value),
	})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	// Update description if provided
	if opts.Description != "" {
		_, err := r.Client.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
			SecretId:    lo.ToPtr(opts.Name),
			Description: lo.ToPtr(opts.Description),
		})
		if err != nil {
			return fmt.Errorf("failed to update description: %w", err)
		}
	}

	// Apply tag changes (additive)
	if opts.TagChange != nil && !opts.TagChange.IsEmpty() {
		if err := tagging.ApplySecret(ctx, r.Client, opts.Name, opts.TagChange); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Updated secret %s (version: %s)\n",
		colors.Success("✓"),
		lo.FromPtr(result.Name),
		lo.FromPtr(result.VersionId),
	)

	return nil
}
