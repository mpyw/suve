package cli

import (
	"context"
	"io"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// deleteRunner executes delete operations using a usecase.
//
//declscope:package // command.go builds and runs it
type deleteRunner struct {
	useCase *stagingusecase.DeleteUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// deleteOptions holds options for the delete command.
//
//declscope:package // command.go fills it from the flags
type deleteOptions struct {
	name           string
	force          bool // For Secrets Manager: force immediate deletion
	recoveryWindow int  // For Secrets Manager: days before permanent deletion (7-30)
	// namespace is the App Configuration namespace of the setting (empty for the
	// null/default namespace and every other provider).
	namespace string
}

// run executes the delete command.
//
//declscope:package // command.go runs it
func (r *deleteRunner) run(ctx context.Context, opts deleteOptions) error {
	result, err := r.useCase.Execute(ctx, stagingusecase.DeleteInput{
		Key:            staging.EntryKey{Name: opts.name, Namespace: opts.namespace},
		Force:          opts.force,
		RecoveryWindow: opts.recoveryWindow,
	})
	if err != nil {
		return err
	}

	// Handle CREATE -> NotStaged (unstage instead of delete)
	if result.Unstaged {
		output.Success(r.stdout, "Unstaged creation: %s", result.Name)

		return nil
	}

	if result.ShowDeleteOptions {
		if result.Force {
			output.Success(r.stdout, "Staged for immediate deletion: %s", result.Name)
		} else {
			output.Success(r.stdout, "Staged for deletion (%d-day recovery): %s", result.RecoveryWindow, result.Name)
		}
	} else {
		output.Success(r.stdout, "Staged for deletion: %s", result.Name)
	}

	return nil
}
