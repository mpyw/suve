// Package restore provides the Secrets Manager restore command.
package restore

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// Runner executes the restore command.
type Runner struct {
	UseCase *secret.RestoreUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the restore command.
type Options struct {
	Name string
}

// Command returns the restore command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "restore",
		Usage:     "Restore a deleted secret",
		ArgsUsage: "<name>",
		Description: `Restore a secret that was scheduled for deletion.

This only works for secrets that were deleted with a recovery window
and haven't been permanently deleted yet. Secrets deleted with --force
cannot be restored.

EXAMPLES:
   suve secret restore my-secret    Restore a deleted secret`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve secret restore <name>")
	}

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		UseCase: &secret.RestoreUseCase{Client: client},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name: cmd.Args().First(),
	})
}

// Run executes the restore command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, secret.RestoreInput{
		Name: opts.Name,
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Restored secret %s\n",
		colors.Success("✓"),
		result.Name,
	)

	return nil
}
