// tag.go provides the generic tag command shared by every provider; untag.go
// holds its untag counterpart over the same TagConfig and TagRunner.
//
// The command scaffolding (argument validation, tag parsing, provider wiring)
// lives here and is identical across providers; only the small per-provider
// TagConfig (help text, resource noun, and provider.Tagger construction) varies.

package generic

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider"
)

// TagConfig holds the provider-specific configuration for the tag/untag commands.
type TagConfig struct {
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

// TagRunner executes the tag/untag commands over a provider.Tagger.
type TagRunner struct {
	Tagger provider.Tagger
	Noun   string
	Stdout io.Writer
}

// RunTag adds or updates the given tags on the named resource.
func (r *TagRunner) RunTag(ctx context.Context, name string, tags map[string]string) error {
	if len(tags) > 0 {
		if err := r.Tagger.Tag(ctx, name, tags); err != nil {
			return fmt.Errorf("failed to add tags: %w", err)
		}
	}

	output.Success(r.Stdout, "Tagged %s %s (%d tag(s))", r.Noun, name, len(tags))

	return nil
}

// TagCommand returns the generic tag command wired with the provider TagConfig.
func TagCommand(cfg TagConfig) *cli.Command {
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

			r := &TagRunner{Tagger: tagger, Noun: cfg.Noun, Stdout: cmd.Root().Writer}

			return r.RunTag(ctx, name, tags)
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
