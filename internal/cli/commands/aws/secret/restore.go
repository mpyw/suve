package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// RestoreRunner executes the restore command.
type RestoreRunner struct {
	UseCase *secret.RestoreUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// RestoreOptions holds the options for the restore command.
type RestoreOptions struct {
	Name string
}

// RestoreCommand returns the restore command.
func RestoreCommand() *cli.Command {
	return &cli.Command{
		Name:      "restore",
		Usage:     "Restore a deleted secret",
		ArgsUsage: argsUsageName,
		Description: `Restore a secret that was scheduled for deletion.

This only works for secrets that were deleted with a recovery window
and haven't been permanently deleted yet. Secrets deleted with --force
cannot be restored.

EXAMPLES:
   suve aws secret restore my-secret    Restore a deleted secret`,
		Action: restoreAction,
	}
}

func restoreAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve aws secret restore <name>")
	}

	store, err := cliinternal.AWSSecretStore(ctx)
	if err != nil {
		return err
	}

	restorer, ok := store.(provider.Restorer)
	if !ok {
		return fmt.Errorf("restore is not supported by this provider")
	}

	r := &RestoreRunner{
		UseCase: &secret.RestoreUseCase{Restorer: restorer},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, RestoreOptions{
		Name: cmd.Args().First(),
	})
}

// Run executes the restore command.
func (r *RestoreRunner) Run(ctx context.Context, opts RestoreOptions) error {
	result, err := r.UseCase.Execute(ctx, secret.RestoreInput{
		Name: opts.Name,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Restored secret %s", result.Name)

	return nil
}
