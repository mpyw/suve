// Package delete provides the SSM stage delete command for staging parameter deletions.
package delete

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/stage"
)

// Runner executes the delete command.
type Runner struct {
	Store  *stage.Store
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the delete command.
type Options struct {
	Name string
}

// Command returns the delete command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Stage a parameter for deletion",
		ArgsUsage: "<name>",
		Description: `Stage a parameter for deletion.

The parameter will be deleted from AWS when you run 'suve ssm stage push'.
Use 'suve ssm stage status' to view staged changes.
Use 'suve ssm stage reset <name>' to unstage.

EXAMPLES:
   suve ssm stage delete /app/old-config  Stage parameter for deletion`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm stage delete <name>")
	}

	store, err := stage.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	r := &Runner{
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{Name: cmd.Args().First()})
}

// Run executes the delete staging.
func (r *Runner) Run(_ context.Context, opts Options) error {
	if err := r.Store.Stage(stage.ServiceSSM, opts.Name, stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
	}); err != nil {
		return err
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Staged for deletion: %s\n", green("✓"), opts.Name)
	return nil
}
