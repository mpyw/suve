// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"fmt"
	"io"

	"github.com/mpyw/suve/internal/cli/colors"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// DeleteRunner executes delete operations using a usecase.
type DeleteRunner struct {
	UseCase *stagingusecase.DeleteUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// DeleteOptions holds options for the delete command.
type DeleteOptions struct {
	Name           string
	Force          bool // For Secrets Manager: force immediate deletion
	RecoveryWindow int  // For Secrets Manager: days before permanent deletion (7-30)
}

// Run executes the delete command.
func (r *DeleteRunner) Run(ctx context.Context, opts DeleteOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.DeleteInput{
		Name:           opts.Name,
		Force:          opts.Force,
		RecoveryWindow: opts.RecoveryWindow,
	})
	if err != nil {
		return err
	}

	if result.ShowDeleteOptions {
		if result.Force {
			_, _ = fmt.Fprintf(r.Stdout, "%s Staged for immediate deletion: %s\n", colors.Success("✓"), result.Name)
		} else {
			_, _ = fmt.Fprintf(r.Stdout, "%s Staged for deletion (%d-day recovery): %s\n", colors.Success("✓"), result.RecoveryWindow, result.Name)
		}
	} else {
		_, _ = fmt.Fprintf(r.Stdout, "%s Staged for deletion: %s\n", colors.Success("✓"), result.Name)
	}
	return nil
}
