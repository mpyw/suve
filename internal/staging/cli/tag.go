package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/maputil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// TagRunner executes tag staging operations using a usecase.
type TagRunner struct {
	UseCase *stagingusecase.TagUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// TagOptions holds options for the tag command.
type TagOptions struct {
	Name string
	Tags []string // key=value pairs to add
}

// Run executes the tag command.
func (r *TagRunner) Run(ctx context.Context, opts TagOptions) error {
	tags, err := parseTags(opts.Tags)
	if err != nil {
		return err
	}

	result, err := r.UseCase.Tag(ctx, stagingusecase.TagInput{
		Name: opts.Name,
		Tags: tags,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Staged tags for: %s", result.Name)
	return nil
}

// UntagRunner executes untag staging operations using a usecase.
type UntagRunner struct {
	UseCase *stagingusecase.TagUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// UntagOptions holds options for the untag command.
type UntagOptions struct {
	Name string
	Keys []string // tag keys to remove
}

// Run executes the untag command.
func (r *UntagRunner) Run(ctx context.Context, opts UntagOptions) error {
	result, err := r.UseCase.Untag(ctx, stagingusecase.UntagInput{
		Name:    opts.Name,
		TagKeys: maputil.NewSet(opts.Keys...),
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Staged tag removal for: %s", result.Name)
	return nil
}

// parseTags parses key=value pairs into a map.
func parseTags(tagSlice []string) (map[string]string, error) {
	tags := make(map[string]string)
	if len(tagSlice) == 0 {
		return tags, nil
	}

	for _, t := range tagSlice {
		parts := strings.SplitN(t, "=", 2) //nolint:mnd // 2 parts: key=value
		if len(parts) != 2 {                 //nolint:mnd // 2 parts expected
			return nil, fmt.Errorf("invalid tag format %q: expected key=value", t)
		}

		tags[parts[0]] = parts[1]
	}
	return tags, nil
}
