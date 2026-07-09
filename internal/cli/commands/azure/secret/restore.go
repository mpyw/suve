package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/usecase/azure"
)

// RestoreRunner executes the restore command.
type RestoreRunner struct {
	UseCase *azure.RestoreUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// RestoreOptions holds the options for the restore command.
type RestoreOptions struct {
	Name string
}

// RestoreCommand returns the Azure Key Vault restore command.
func RestoreCommand() *cli.Command {
	return &cli.Command{
		Name:      "restore",
		Usage:     "Restore a soft-deleted secret",
		ArgsUsage: argsUsageName,
		Description: `Recover a soft-deleted Key Vault secret (RecoverDeletedSecret).

Works while the secret is within the vault's soft-delete retention window and
has not been purged. A secret deleted with --force (purged) cannot be restored.

EXAMPLES:
   suve azure secret restore my-secret    Recover a soft-deleted secret`,
		Action: restoreAction,
	}
}

func restoreAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve azure secret restore <name>")
	}

	store, err := cliinternal.AzureKeyVaultStore(ctx)
	if err != nil {
		return err
	}

	restorer, ok := store.(provider.Restorer)
	if !ok {
		return fmt.Errorf("restore is not supported by this provider")
	}

	r := &RestoreRunner{
		UseCase: &azure.RestoreUseCase{Restorer: restorer},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, RestoreOptions{Name: cmd.Args().First()})
}

// Run executes the restore command.
func (r *RestoreRunner) Run(ctx context.Context, opts RestoreOptions) error {
	result, err := r.UseCase.Execute(ctx, azure.RestoreInput{Name: opts.Name})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Restored secret %s", result.Name)

	return nil
}
