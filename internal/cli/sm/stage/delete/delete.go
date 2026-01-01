// Package delete provides the SM stage delete command for staging secret deletions.
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
	Name           string
	Force          bool
	RecoveryWindow int
}

// Command returns the delete command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Stage a secret for deletion",
		ArgsUsage: "<name>",
		Description: `Stage a secret for deletion.

The secret will be deleted from AWS when you run 'suve sm stage push'.
Use 'suve sm stage status' to view staged changes.
Use 'suve sm stage reset <name>' to unstage.

RECOVERY WINDOW:
   By default, secrets are scheduled for deletion after a 30-day recovery window.
   During this period, you can restore the secret using 'suve sm restore'.
   Use --force for immediate permanent deletion without recovery.

   Minimum: 7 days
   Maximum: 30 days
   Default: 30 days

EXAMPLES:
   suve sm stage delete my-secret                      Stage with 30-day recovery
   suve sm stage delete --recovery-window 7 my-secret  Stage with 7-day recovery
   suve sm stage delete --force my-secret              Stage for immediate deletion`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Force immediate deletion without recovery window",
			},
			&cli.IntFlag{
				Name:  "recovery-window",
				Usage: "Number of days before permanent deletion (7-30)",
				Value: 30,
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve sm stage delete <name>")
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

	opts := Options{
		Name:           cmd.Args().First(),
		Force:          cmd.Bool("force"),
		RecoveryWindow: int(cmd.Int("recovery-window")),
	}

	// Validate recovery window
	if !opts.Force && (opts.RecoveryWindow < 7 || opts.RecoveryWindow > 30) {
		return fmt.Errorf("recovery window must be between 7 and 30 days")
	}

	return r.Run(ctx, opts)
}

// Run executes the delete staging.
func (r *Runner) Run(_ context.Context, opts Options) error {
	entry := stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &stage.DeleteOptions{
			Force:          opts.Force,
			RecoveryWindow: opts.RecoveryWindow,
		},
	}

	if err := r.Store.Stage(stage.ServiceSM, opts.Name, entry); err != nil {
		return err
	}

	red := color.New(color.FgRed).SprintFunc()
	if opts.Force {
		_, _ = fmt.Fprintf(r.Stdout, "%s Staged for immediate deletion: %s\n", red("✗"), opts.Name)
	} else {
		_, _ = fmt.Fprintf(r.Stdout, "%s Staged for deletion (%d-day recovery): %s\n", red("✗"), opts.RecoveryWindow, opts.Name)
	}
	return nil
}
