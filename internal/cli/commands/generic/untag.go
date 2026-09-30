package generic

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
)

// runUntag removes the tags with the given keys from the named resource.
func (r *tagRunner) runUntag(ctx context.Context, name string, keys []string) error {
	if len(keys) > 0 {
		if err := r.tagger.Untag(ctx, name, keys); err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	output.Success(r.stdout, "Untagged %s %s (%d key(s))", r.noun, name, len(keys))

	return nil
}

// UntagCommand returns the generic untag command wired with the provider TagConfig.
func UntagCommand(cfg TagConfig) *cli.Command {
	return &cli.Command{
		Name:        "untag",
		Usage:       cfg.Usage,
		ArgsUsage:   cfg.ArgsUsage,
		Description: cfg.Description,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: name and key
				return fmt.Errorf("%s", cfg.UsageError)
			}

			name := cmd.Args().Get(0)
			keys := cmd.Args().Slice()[1:]

			tagger, err := cfg.NewTagger(ctx)
			if err != nil {
				return err
			}

			r := &tagRunner{tagger: tagger, noun: cfg.Noun, stdout: cmd.Root().Writer}

			return r.runUntag(ctx, name, keys)
		},
	}
}
