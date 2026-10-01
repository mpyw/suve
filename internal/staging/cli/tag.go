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

// tagRunner executes tag staging operations using a usecase.
//
//declscope:package // command.go builds and runs it
type tagRunner struct {
	useCase *stagingusecase.TagUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// tagOptions holds options for the tag command.
//
//declscope:package // command.go fills it from the flags
type tagOptions struct {
	name string
	// namespace is the App Configuration namespace of the resource (empty for the
	// null/default namespace and every other provider).
	namespace string
	tags      []string // key=value pairs to add
}

// run executes the tag command.
//
//declscope:package // command.go runs it
func (r *tagRunner) run(ctx context.Context, opts tagOptions) error {
	tags, err := parseTags(opts.tags)
	if err != nil {
		return err
	}

	result, err := r.useCase.Tag(ctx, stagingusecase.TagInput{
		Key:  staging.EntryKey{Name: opts.name, Namespace: opts.namespace},
		Tags: tags,
	})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Staged tags for: %s", result.Name)

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
