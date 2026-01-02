// Package reset provides the global reset command for unstaging all changes.
package reset

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/staging"
)

// Runner executes the reset command.
type Runner struct {
	Store  *staging.Store
	Stdout io.Writer
	Stderr io.Writer
}

// Command returns the global reset command.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "reset",
		Usage: "Unstage all changes",
		Description: `Remove all staged changes (SSM Parameter Store and Secrets Manager) from the staging area.

This does not affect AWS - it only clears the local staging area.

Use 'suve stage param reset' or 'suve stage secret reset' for service-specific operations.

EXAMPLES:
   suve stage reset --all    Unstage all changes (SSM Parameter Store and Secrets Manager)`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Unstage all changes (required)",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	// Require --all flag for safety
	if !cmd.Bool("all") {
		_, _ = fmt.Fprintln(cmd.Root().ErrWriter, colors.Warning("Warning: no effect without --all flag"))
		_, _ = fmt.Fprintln(cmd.Root().ErrWriter, "Hint: Use 'suve stage reset --all' to unstage all changes")
		return nil
	}

	store, err := staging.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize stage store: %w", err)
	}

	r := &Runner{
		Store:  store,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	return r.Run(ctx)
}

// Run executes the reset command.
func (r *Runner) Run(_ context.Context) error {
	// Get counts before reset
	staged, err := r.Store.List("")
	if err != nil {
		return err
	}

	paramCount := len(staged[staging.ServiceParam])
	secretCount := len(staged[staging.ServiceSecret])
	totalCount := paramCount + secretCount

	if totalCount == 0 {
		_, _ = fmt.Fprintln(r.Stdout, colors.Warning("No changes staged."))
		return nil
	}

	// Unstage all SSM Parameter Store
	if paramCount > 0 {
		if err := r.Store.UnstageAll(staging.ServiceParam); err != nil {
			return err
		}
	}

	// Unstage all Secrets Manager
	if secretCount > 0 {
		if err := r.Store.UnstageAll(staging.ServiceSecret); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged all changes (%d SSM Parameter Store, %d Secrets Manager)\n",
		colors.Success("✓"), paramCount, secretCount)
	return nil
}
