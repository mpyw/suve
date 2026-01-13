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

// PersistRunner executes persist operations using a usecase.
type PersistRunner struct {
	UseCase   *stagingusecase.PersistUseCase
	Stdout    io.Writer
	Stderr    io.Writer
	Encrypted bool // Whether the file is encrypted (for output messages)
}

// PersistOptions holds options for the persist command.
type PersistOptions struct {
	// Service filters the persist to a specific service. Empty means all services.
	Service staging.Service
	// Keep preserves the agent memory after persisting.
	Keep bool
}

// Run executes the persist command.
func (r *PersistRunner) Run(ctx context.Context, opts PersistOptions) error {
	_, err := r.UseCase.Execute(ctx, stagingusecase.PersistInput{
		Service: opts.Service,
		Keep:    opts.Keep,
	})

	if err != nil {
		// Check for non-fatal error (state was written but agent cleanup failed)
		var persistErr *stagingusecase.PersistError
		if errors.As(err, &persistErr) && persistErr.NonFatal {
			output.Warn(r.Stderr, "Warning: %v", err)
			// Continue with success message since state was written
		} else {
			return err
		}
	}

	// Output success message
	if opts.Keep {
		if r.Encrypted {
			output.Success(r.Stdout, "Staged changes persisted to file (encrypted, kept in memory)")
		} else {
			output.Success(r.Stdout, "Staged changes persisted to file (kept in memory)")
		}
	} else {
		if r.Encrypted {
			output.Success(r.Stdout, "Staged changes persisted to file (encrypted) and cleared from memory")
		} else {
			output.Success(r.Stdout, "Staged changes persisted to file and cleared from memory")
		}
	}

	// Display warning about plain-text storage only if not encrypted
	if !r.Encrypted {
		output.Warn(r.Stderr, "Note: secrets are stored as plain text.")
	}

	return nil
}
