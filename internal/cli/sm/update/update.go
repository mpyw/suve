// Package update provides the SM update command.
package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/confirm"
)

// Client is the interface for the update command.
type Client interface {
	smapi.PutSecretValueAPI
	smapi.UpdateSecretAPI
	smapi.TagResourceAPI
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
	Tags        map[string]string
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
  suve sm update my-config '{"host":"new-db.com"}'     Update JSON secret
  suve sm update -y my-api-key "new-key-value"         Update without confirmation`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: "Update secret description",
			},
			&cli.StringSliceFlag{
				Name:  "tag",
				Usage: "Tag in key=value format (can be specified multiple times)",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve sm update <name> <value>")
	}

	name := cmd.Args().Get(0)
	skipConfirm := cmd.Bool("yes")

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

	client, err := awsutil.NewSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	tags, err := parseTags(cmd.StringSlice("tag"))
	if err != nil {
		return err
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
		Tags:        tags,
	})
}

func parseTags(tagSlice []string) (map[string]string, error) {
	if len(tagSlice) == 0 {
		return nil, nil
	}
	tags := make(map[string]string)
	for _, t := range tagSlice {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format %q: expected key=value", t)
		}
		tags[parts[0]] = parts[1]
	}
	return tags, nil
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

	// Update tags if provided
	if len(opts.Tags) > 0 {
		smTags := make([]types.Tag, 0, len(opts.Tags))
		for k, v := range opts.Tags {
			smTags = append(smTags, types.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
		_, err := r.Client.TagResource(ctx, &secretsmanager.TagResourceInput{
			SecretId: lo.ToPtr(opts.Name),
			Tags:     smTags,
		})
		if err != nil {
			return fmt.Errorf("failed to update tags: %w", err)
		}
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Updated secret %s (version: %s)\n",
		green("✓"),
		lo.FromPtr(result.Name),
		lo.FromPtr(result.VersionId),
	)

	return nil
}
