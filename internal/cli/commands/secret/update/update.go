// Package update provides the Secrets Manager update command.
package update

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/tagging"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// Runner executes the update command.
type Runner struct {
	UseCase *secret.UpdateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
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

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	uc := &secret.UpdateUseCase{Client: client}
	newValue := cmd.Args().Get(1)

	// Fetch current value and show diff before confirming
	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			diff := output.Diff(name+" (AWS)", name+" (new)", currentValue, newValue)
			if diff != "" {
				_, _ = fmt.Fprintln(cmd.Root().ErrWriter, diff)
			}
		}

		// Confirm operation
		prompter := &confirm.Prompter{
			Stdin:  os.Stdin,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}
		confirmed, err := prompter.ConfirmAction("Update secret", name, false)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	r := &Runner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name:        name,
		Value:       newValue,
		Description: cmd.String("description"),
		TagChange:   tagResult.Change,
	})
}

// Run executes the update command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, secret.UpdateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Description: opts.Description,
		TagChange:   opts.TagChange,
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Updated secret %s (version: %s)\n",
		colors.Success("✓"),
		result.Name,
		result.VersionID,
	)

	return nil
}
