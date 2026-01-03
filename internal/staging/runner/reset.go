// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"fmt"
	"io"

	"github.com/mpyw/suve/internal/cli/colors"
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
		_, _ = fmt.Fprintf(r.Stdout, "%s\n", colors.Warning(fmt.Sprintf("No %s changes staged.", result.ServiceName)))
	case stagingusecase.ResetResultUnstagedAll:
		_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged all %s %ss (%d)\n", colors.Success("✓"), result.ServiceName, result.ItemName, result.Count)
	case stagingusecase.ResetResultNotStaged:
		_, _ = fmt.Fprintf(r.Stdout, "%s %s is not staged\n", colors.Warning("!"), result.Name)
	case stagingusecase.ResetResultUnstaged:
		_, _ = fmt.Fprintf(r.Stdout, "%s Unstaged %s\n", colors.Success("✓"), result.Name)
	case stagingusecase.ResetResultRestored:
		_, _ = fmt.Fprintf(r.Stdout, "%s Restored %s (staged from version %s)\n",
			colors.Success("✓"), result.Name, result.VersionLabel)
	}

	return nil
}
