package cli

import (
	"context"
	"io"

	"github.com/mpyw/suve/internal/cli/output"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// resetRunner executes reset operations using a usecase.
//
//declscope:shared // command.go builds and runs it
type resetRunner struct {
	useCase *stagingusecase.ResetUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// resetOptions holds options for the reset command.
//
//declscope:shared // command.go fills it from the flags
type resetOptions struct {
	spec string // Name with optional version spec
	all  bool   // Reset all staged items for this service
	// namespace is the App Configuration namespace of the entry to reset (empty
	// for the null/default namespace and every other provider; ignored with All).
	namespace string
}

// run executes the reset command.
//
//declscope:shared // command.go runs it
func (r *resetRunner) run(ctx context.Context, opts resetOptions) error {
	result, err := r.useCase.Execute(ctx, stagingusecase.ResetInput{
		Spec:      opts.spec,
		All:       opts.all,
		Namespace: opts.namespace,
	})
	if err != nil {
		return err
	}

	switch result.Type {
	case stagingusecase.ResetResultNothingStaged:
		output.Info(r.stdout, "No %s changes staged.", result.ServiceName)
	case stagingusecase.ResetResultUnstagedAll:
		output.Success(r.stdout, "Unstaged all %s %ss (%d)", result.ServiceName, result.ItemName, result.Count)
	case stagingusecase.ResetResultNotStaged:
		output.Warn(r.stdout, "%s is not staged", result.Name)
	case stagingusecase.ResetResultUnstaged:
		output.Success(r.stdout, "Unstaged %s", result.Name)
	case stagingusecase.ResetResultUnstagedTag:
		output.Success(r.stdout, "Unstaged tag changes for %s", result.Name)
	case stagingusecase.ResetResultRestored:
		output.Success(r.stdout, "Restored %s (staged from version %s)", result.Name, result.VersionLabel)
	case stagingusecase.ResetResultSkipped:
		output.Warn(r.stdout, "Skipped %s (version %s matches current value)", result.Name, result.VersionLabel)
	}

	return nil
}
