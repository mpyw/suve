// Package cli provides shared runners and command builders for stage commands.
package cli

import (
	"context"
	"io"

	"github.com/mpyw/suve/internal/cli/output"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// ResetRunner executes reset operations using a usecase.
type ResetRunner struct {
	UseCase *stagingusecase.ResetUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// ResetOptions holds options for the reset command.
type ResetOptions struct {
	Spec string // Name with optional version spec
	All  bool   // Reset all staged items for this service
}

// Run executes the reset command.
func (r *ResetRunner) Run(ctx context.Context, opts ResetOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.ResetInput{
		Spec: opts.Spec,
		All:  opts.All,
	})
	if err != nil {
		return err
	}

	switch result.Type {
	case stagingusecase.ResetResultNothingStaged:
		output.Info(r.Stdout, "No %s changes staged.", result.ServiceName)
	case stagingusecase.ResetResultUnstagedAll:
		output.Success(r.Stdout, "Unstaged all %s %ss (%d)", result.ServiceName, result.ItemName, result.Count)
	case stagingusecase.ResetResultNotStaged:
		output.Warn(r.Stdout, "%s is not staged", result.Name)
	case stagingusecase.ResetResultUnstaged:
		output.Success(r.Stdout, "Unstaged %s", result.Name)
	case stagingusecase.ResetResultRestored:
		output.Success(r.Stdout, "Restored %s (staged from version %s)", result.Name, result.VersionLabel)
	case stagingusecase.ResetResultSkipped:
		output.Warn(r.Stdout, "Skipped %s (version %s matches current value)", result.Name, result.VersionLabel)
	}

	return nil
}
