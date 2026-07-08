package gcloud

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/gcloud"
)

// UpdateRunner executes the update command.
type UpdateRunner struct {
	UseCase *gcloud.UpdateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// UpdateOptions holds the options for the update command.
type UpdateOptions struct {
	Name  string
	Value string
}

// UpdateCommand returns the Google Cloud Secret Manager update command.
func UpdateCommand() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a secret value",
		ArgsUsage: "<name> <value>",
		Description: `Update the value of an existing secret by adding a new version.

The new version becomes the latest; prior versions remain accessible by number.
Use 'suve gcloud secret create' to create a new secret.

EXAMPLES:
  suve gcloud secret update my-api-key "new-value"       Add a new version
  suve gcloud secret update --yes my-api-key "new-value" Update without confirmation`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: updateAction,
	}
}

func updateAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: name and value
		return fmt.Errorf("usage: suve gcloud secret update <name> <value>")
	}

	name := cmd.Args().Get(0)
	newValue := cmd.Args().Get(1)
	skipConfirm := cmd.Bool("yes")

	store, err := cliinternal.GoogleCloudSecretStore(ctx)
	if err != nil {
		return err
	}

	uc := &gcloud.UpdateUseCase{Store: store}

	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			diff := output.Diff(cmd.Root().ErrWriter, name+" (current)", name+" (new)", currentValue, newValue)
			if diff != "" {
				output.Println(cmd.Root().ErrWriter, diff)
			}
		}

		prompter := &confirm.Prompter{
			Stdin:  os.Stdin,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}

		confirmed, cerr := prompter.ConfirmAction("Update secret", name, false)
		if cerr != nil {
			return cerr
		}

		if !confirmed {
			return nil
		}
	}

	r := &UpdateRunner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, UpdateOptions{Name: name, Value: newValue})
}

// Run executes the update command.
func (r *UpdateRunner) Run(ctx context.Context, opts UpdateOptions) error {
	result, err := r.UseCase.Execute(ctx, gcloud.UpdateInput{Name: opts.Name, Value: opts.Value})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Updated secret %s (version: %s)", result.Name, result.Version)

	return nil
}
