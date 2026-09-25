package secret

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// DeleteRunner executes the delete command.
type DeleteRunner struct {
	UseCase *secret.DeleteUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// DeleteOptions holds the options for the delete command.
type DeleteOptions struct {
	Name string
}

// DeleteCommand returns the Azure Key Vault delete command.
func DeleteCommand() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Aliases:   []string{"rm"},
		Usage:     "Delete a secret",
		ArgsUsage: argsUsageName,
		Description: `Delete a secret and all its versions from Azure Key Vault.

When the vault has soft-delete enabled, the secret is recoverable within the
vault's retention window (use 'suve azure secret restore'); otherwise deletion
is permanent.

EXAMPLES:
   suve azure secret delete my-secret          Soft-delete (with confirmation)
   suve azure secret delete --yes my-secret    Soft-delete without confirmation`,
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
		return fmt.Errorf("usage: suve azure secret delete <name>")
	}

	name := cmd.Args().First()
	skipConfirm := cmd.Bool("yes")

	store, err := azureinternal.KeyVaultStore(ctx)
	if err != nil {
		return err
	}

	uc := &secret.DeleteUseCase{Store: store}

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
		Target: azureinternal.KeyVaultConfirmTarget(ctx),
	}

	confirmed, err := prompter.ConfirmRecoverableDelete(
		name, "recoverable with 'suve azure secret restore' while the vault keeps soft-deleted secrets", skipConfirm,
	)
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
	result, err := r.UseCase.Execute(ctx, secret.DeleteInput{Name: opts.Name})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Deleted secret %s", result.Name)

	return nil
}
