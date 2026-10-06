package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// restoreRunner executes the restore command.
type restoreRunner struct {
	useCase *secret.RestoreUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// restoreOptions holds the options for the restore command.
type restoreOptions struct {
	name string
}

// restoreCommand returns the Azure Key Vault restore command.
//
//declscope:shared // command.go registers it
func restoreCommand() *cli.Command {
	return &cli.Command{
		Name:      "restore",
		Usage:     "Restore a soft-deleted secret",
		ArgsUsage: argsUsageName,
		Description: `Recover a soft-deleted Key Vault secret (RecoverDeletedSecret).

Works while the secret is within the vault's soft-delete retention window and
has not been purged.

EXAMPLES:
   suve azure secret restore my-secret    Recover a soft-deleted secret`,
		Action: restoreAction,
	}
}

func restoreAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve azure secret restore <name>")
	}

	store, err := azureinternal.KeyVaultStore(ctx)
	if err != nil {
		return err
	}

	restorer, ok := store.(provider.Restorer)
	if !ok {
		return fmt.Errorf("restore is not supported by this provider")
	}

	r := &restoreRunner{
		useCase: &secret.RestoreUseCase{Restorer: restorer},
		stdout:  cmd.Root().Writer,
		stderr:  cmd.Root().ErrWriter,
	}

	return r.run(ctx, restoreOptions{name: cmd.Args().First()})
}

// run executes the restore command.
func (r *restoreRunner) run(ctx context.Context, opts restoreOptions) error {
	result, err := r.useCase.Execute(ctx, secret.RestoreInput{Name: opts.name})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Restored secret %s", result.Name)

	return nil
}
