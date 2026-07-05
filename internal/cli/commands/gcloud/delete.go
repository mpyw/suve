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

// DeleteRunner executes the delete command.
type DeleteRunner struct {
	UseCase *gcloud.DeleteUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// DeleteOptions holds the options for the delete command.
type DeleteOptions struct {
	Name string
}

// DeleteCommand returns the Google Cloud Secret Manager delete command.
func DeleteCommand() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Aliases:   []string{"rm"},
		Usage:     "Delete a secret",
		ArgsUsage: "<name>",
		Description: `Permanently delete a secret and all its versions from Google Cloud
Secret Manager.

WARNING: Unlike AWS Secrets Manager, Google Cloud has NO recovery window.
Deletion is immediate and permanent; there is no restore command.

EXAMPLES:
   suve gcloud secret delete my-secret        Delete (with confirmation)
   suve gcloud secret delete --yes my-secret  Delete without confirmation`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: deleteAction,
	}
}

func deleteAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve gcloud secret delete <name>")
	}

	name := cmd.Args().First()
	skipConfirm := cmd.Bool("yes")

	store, err := cliinternal.GoogleCloudSecretStore(ctx)
	if err != nil {
		return err
	}

	uc := &gcloud.DeleteUseCase{Store: store}

	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			output.Info(cmd.Root().ErrWriter, "Current value of %s:", name)
			output.Println(cmd.Root().ErrWriter, "")
			output.Println(cmd.Root().ErrWriter, output.Indent(currentValue, "  "))
			output.Println(cmd.Root().ErrWriter, "")
		}
	}

	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	confirmed, err := prompter.ConfirmDelete(name, skipConfirm)
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	r := &DeleteRunner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, DeleteOptions{Name: name})
}

// Run executes the delete command.
func (r *DeleteRunner) Run(ctx context.Context, opts DeleteOptions) error {
	result, err := r.UseCase.Execute(ctx, gcloud.DeleteInput{Name: opts.Name})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Permanently deleted secret %s", result.Name)

	return nil
}
