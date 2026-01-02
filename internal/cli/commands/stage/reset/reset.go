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
		Description: `Remove all staged changes (both SSM and SM) from the staging area.

This does not affect AWS - it only clears the local staging area.

Use 'suve stage param reset' or 'suve stage secret reset' for service-specific operations.

EXAMPLES:
   suve stage reset    Unstage all changes (SSM and SM)`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
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

	ssmCount := len(staged[staging.ServiceParam])
	smCount := len(staged[staging.ServiceSecret])
	totalCount := ssmCount + smCount

	if totalCount == 0 {
		_, _ = fmt.Fprintln(r.Stdout, colors.Warning("No changes staged."))
		return nil
	}

	// Unstage all SSM
	if ssmCount > 0 {
		if err := r.Store.UnstageAll(staging.ServiceParam); err != nil {
			return err
		}
	}

	// Unstage all SM
	if smCount > 0 {
		if err := r.Store.UnstageAll(staging.ServiceSecret); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged all changes (%d SSM, %d SM)\n",
		colors.Success("✓"), ssmCount, smCount)
	return nil
}
