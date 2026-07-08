package param

import (
	"context"

	"github.com/urfave/cli/v3"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the Azure App Configuration provider.Tagger. The adapter
// writes tags via a GET-merge-PUT with an ETag precondition (azappconfig/v2), so
// the value and any other tags are preserved.
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return cliinternal.AzureAppConfigStore(ctx)
}

// TagCommand returns the Azure App Configuration tag command.
func TagCommand() *cli.Command {
	return generictag.TagCommand(generictag.Config{
		Usage:     "Add or update tags on a setting",
		ArgsUsage: "<key> <key=value>...",
		Description: `Add or update tags on a setting.

Tags are written with a GET-merge-PUT (App Configuration replaces the whole
key-value, so the current value and any other tags are re-sent unchanged); an
ETag precondition guards against a concurrent write.

EXAMPLES:
   suve azure param tag app/timeout env=prod                 Add or update the env tag`,
		Noun:       "setting",
		UsageError: "usage: suve azure param tag <key> <key=value> [key=value]",
		NewTagger:  newTagger,
	})
}

// UntagCommand returns the Azure App Configuration untag command.
func UntagCommand() *cli.Command {
	return generictag.UntagCommand(generictag.Config{
		Usage:     "Remove tags from a setting",
		ArgsUsage: "<key> <key>...",
		Description: `Remove tags from a setting.

Tags are removed with a GET-merge-PUT (the current value and any remaining tags
are preserved); an ETag precondition guards against a concurrent write.

EXAMPLES:
   suve azure param untag app/timeout env                    Remove the env tag`,
		Noun:       "setting",
		UsageError: "usage: suve azure param untag <key> <key> [key]",
		NewTagger:  newTagger,
	})
}
