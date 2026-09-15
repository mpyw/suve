package cli

import (
	"context"
	"io"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// UntagRunner executes untag staging operations using a usecase.
type UntagRunner struct {
	UseCase *stagingusecase.TagUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// UntagOptions holds options for the untag command.
type UntagOptions struct {
	Name string
	// Namespace is the App Configuration namespace of the resource (empty for the
	// null/default namespace and every other provider).
	Namespace string
	Keys      []string // tag keys to remove
}

// Run executes the untag command.
func (r *UntagRunner) Run(ctx context.Context, opts UntagOptions) error {
	result, err := r.UseCase.Untag(ctx, stagingusecase.UntagInput{
		Key:     staging.EntryKey{Name: opts.Name, Namespace: opts.Namespace},
		TagKeys: maputil.NewSet(opts.Keys...),
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Staged tag removal for: %s", result.Name)

	return nil
}
