package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
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

	store, err := awsinternal.SecretStore(ctx)
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

	return r.run(ctx, restoreOptions{
		name: cmd.Args().First(),
	})
}

// run executes the restore command.
func (r *restoreRunner) run(ctx context.Context, opts restoreOptions) error {
	result, err := r.useCase.Execute(ctx, secret.RestoreInput{
		Name: opts.name,
	})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Restored secret %s", result.Name)

	return nil
}
