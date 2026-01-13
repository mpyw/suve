// Package cli provides shared runners and command builders for stage commands.
package cli

import (
	"context"
	"errors"
	"io"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// DrainRunner executes drain operations using a usecase.
type DrainRunner struct {
	UseCase *stagingusecase.DrainUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// DrainOptions holds options for the drain command.
type DrainOptions struct {
	// Service filters the drain to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the file after draining.
	Keep bool
	// Force overwrites agent memory without checking for conflicts.
	Force bool
	// Merge combines file changes with existing agent memory.
	Merge bool
}

// Run executes the drain command.
func (r *DrainRunner) Run(ctx context.Context, opts DrainOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.DrainInput{
		Service: opts.Service,
		Keep:    opts.Keep,
		Force:   opts.Force,
		Merge:   opts.Merge,
	})

	if err != nil {
		// Check for non-fatal error (state was written but file cleanup failed)
		var drainErr *stagingusecase.DrainError
		if errors.As(err, &drainErr) && drainErr.NonFatal {
			output.Warn(r.Stderr, "Warning: %v", err)
			// Continue with success message since state was written
		} else {
			return err
		}
	}

	// Output success message
	if result.Merged {
		if opts.Keep {
			output.Success(r.Stdout, "Staged changes loaded and merged from file (file kept)")
		} else {
			output.Success(r.Stdout, "Staged changes loaded and merged from file (file deleted)")
		}
	} else {
		if opts.Keep {
			output.Success(r.Stdout, "Staged changes loaded from file (file kept)")
		} else {
			output.Success(r.Stdout, "Staged changes loaded from file and file deleted")
		}
	}

	return nil
}
