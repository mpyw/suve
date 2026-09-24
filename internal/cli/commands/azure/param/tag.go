package param

import (
	"context"

	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the Azure App Configuration provider.Tagger. The adapter
// writes tags via a GET-merge-PUT with an ETag precondition (azappconfig/v2), so
// the value and any other tags are preserved.
//
//declscope:package // untag.go's UntagCommand builds the same Tagger
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return azureinternal.AppConfigStore(ctx)
}

// TagCommand returns the Azure App Configuration tag command.
func TagCommand() *cli.Command {
	return generic.TagCommand(generic.TagConfig{
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
