package cli

import (
	"context"
	"io"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// untagRunner executes untag staging operations using a usecase.
//
//declscope:package // command.go builds and runs it
type untagRunner struct {
	useCase *stagingusecase.TagUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// untagOptions holds options for the untag command.
//
//declscope:package // command.go fills it from the flags
type untagOptions struct {
	name string
	// namespace is the App Configuration namespace of the resource (empty for the
	// null/default namespace and every other provider).
	namespace string
	keys      []string // tag keys to remove
}

// run executes the untag command.
//
//declscope:package // command.go runs it
func (r *untagRunner) run(ctx context.Context, opts untagOptions) error {
	result, err := r.useCase.Untag(ctx, stagingusecase.UntagInput{
		Key:     staging.EntryKey{Name: opts.name, Namespace: opts.namespace},
		TagKeys: maputil.NewSet(opts.keys...),
	})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Staged tag removal for: %s", result.Name)

	return nil
}
