// Package tag provides the generic tag/untag commands shared by every provider
// (AWS SSM Parameter Store, AWS Secrets Manager, and future providers).
//
// The command scaffolding (argument validation, tag parsing, provider wiring)
// lives here and is identical across providers; only the small per-provider
// Config (help text, resource noun, and provider.Tagger construction) varies.
package tag

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
)

// Config holds the provider-specific configuration for the tag/untag commands.
type Config struct {
	// Usage is the one-line command usage string.
	Usage string
	// ArgsUsage is the positional-arguments usage string.
	ArgsUsage string
	// Description is the long help text.
	Description string
	// Noun is the resource word used in success messages ("parameter"/"secret").
	Noun string
	// UsageError is the error returned when required arguments are missing.
	UsageError string
	// NewTagger builds the provider's Tagger from the CLI context.
	NewTagger func(ctx context.Context) (provider.Tagger, error)
}

// Runner executes the tag/untag commands over a provider.Tagger.
type Runner struct {
	Tagger provider.Tagger
	Noun   string
	Stdout io.Writer
}

// RunTag adds or updates the given tags on the named resource.
func (r *Runner) RunTag(ctx context.Context, name string, tags map[string]string) error {
	if len(tags) > 0 {
		if err := r.Tagger.Tag(ctx, name, tags); err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	output.Success(r.Stdout, "Tagged %s %s (%d tag(s))", r.Noun, name, len(tags))

	return nil
}

// RunUntag removes the tags with the given keys from the named resource.
func (r *Runner) RunUntag(ctx context.Context, name string, keys []string) error {
	if len(keys) > 0 {
		if err := r.Tagger.Untag(ctx, name, keys); err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	output.Success(r.Stdout, "Untagged %s %s (%d key(s))", r.Noun, name, len(keys))

	return nil
}

// TagCommand returns the generic tag command wired with the provider Config.
//
//nolint:revive // TagCommand pairs with UntagCommand; the symmetric names read clearer than dropping the prefix
func TagCommand(cfg Config) *cli.Command {
	return &cli.Command{
		Name:        "tag",
		Usage:       cfg.Usage,
		ArgsUsage:   cfg.ArgsUsage,
		Description: cfg.Description,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 2 { //nolint:mnd // minimum required args: name and key=value
				return fmt.Errorf("%s", cfg.UsageError)
			}

			name := cmd.Args().Get(0)

			tags, err := parseTags(cmd.Args().Slice()[1:])
			if err != nil {
				return err
			}

			tagger, err := cfg.NewTagger(ctx)
			if err != nil {
				return err
			}

			r := &Runner{Tagger: tagger, Noun: cfg.Noun, Stdout: cmd.Root().Writer}

			return r.RunTag(ctx, name, tags)
		},
	}
}

// UntagCommand returns the generic untag command wired with the provider Config.
func UntagCommand(cfg Config) *cli.Command {
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

			r := &Runner{Tagger: tagger, Noun: cfg.Noun, Stdout: cmd.Root().Writer}

			return r.RunUntag(ctx, name, keys)
		},
	}
}

// parseTags parses key=value arguments into a tag map. It is the single shared
// implementation that replaced the byte-identical copies in the param and secret
// tag commands.
func parseTags(args []string) (map[string]string, error) {
	tags := make(map[string]string)

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2) //nolint:mnd // split into key=value pair
		if len(parts) != 2 {                 //nolint:mnd // expect exactly key and value
			return nil, fmt.Errorf("invalid tag format %q: expected key=value", arg)
		}

		key, value := parts[0], parts[1]
		if key == "" {
			return nil, fmt.Errorf("invalid tag format %q: key cannot be empty", arg)
		}

		tags[key] = value
	}

	return tags, nil
}
