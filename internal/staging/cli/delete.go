package cli

import (
	"context"
	"io"

	"github.com/mpyw/suve/internal/cli/output"
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

	// Handle CREATE -> NotStaged (unstage instead of delete)
	if result.Unstaged {
		output.Success(r.Stdout, "Unstaged creation: %s", result.Name)
		return nil
	}

	if result.ShowDeleteOptions {
		if result.Force {
			output.Success(r.Stdout, "Staged for immediate deletion: %s", result.Name)
		} else {
			output.Success(r.Stdout, "Staged for deletion (%d-day recovery): %s", result.RecoveryWindow, result.Name)
		}
	} else {
		output.Success(r.Stdout, "Staged for deletion: %s", result.Name)
	}

	return nil
}
