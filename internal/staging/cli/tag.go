package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging"
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
	// Namespace is the App Configuration namespace of the resource (empty for the
	// null/default namespace and every other provider).
	Namespace string
	Tags      []string // key=value pairs to add
}

// Run executes the tag command.
func (r *TagRunner) Run(ctx context.Context, opts TagOptions) error {
	tags, err := parseTags(opts.Tags)
	if err != nil {
		return err
	}

	result, err := r.UseCase.Tag(ctx, stagingusecase.TagInput{
		Key:  staging.EntryKey{Name: opts.Name, Namespace: opts.Namespace},
		Tags: tags,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Staged tags for: %s", result.Name)

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
		if len(parts) != 2 {               //nolint:mnd // 2 parts expected
			return nil, fmt.Errorf("invalid tag format %q: expected key=value", t)
		}

		tags[parts[0]] = parts[1]
	}

	return tags, nil
}
