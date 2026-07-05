package param

import (
	"context"

	"github.com/urfave/cli/v3"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the Azure App Configuration provider.Tagger.
//
// Note: tag mutation is declined by the App Configuration adapter (the
// azappconfig SDK cannot write setting tags without clearing them); these
// commands surface that clear error rather than crash.
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return cliinternal.AzureAppConfigStore(ctx)
}

// TagCommand returns the Azure App Configuration tag command.
func TagCommand() *cli.Command {
	return generictag.TagCommand(generictag.Config{
		Usage:     "Add or update tags on a setting (unsupported)",
		ArgsUsage: "<key> <key=value>...",
		Description: `Add or update tags on a setting.

Note: the azappconfig SDK cannot write setting tags without clearing them, so
this command reports that tag mutation is unsupported rather than losing data.
Tags set out-of-band (e.g. via the Azure portal) are still shown by
'suve azure param show'.

EXAMPLES:
   suve azure param tag app/timeout env=prod                 Reports "tags unsupported"`,
		Noun:       "setting",
		UsageError: "usage: suve azure param tag <key> <key=value> [key=value]",
		NewTagger:  newTagger,
	})
}

// UntagCommand returns the Azure App Configuration untag command.
func UntagCommand() *cli.Command {
	return generictag.UntagCommand(generictag.Config{
		Usage:     "Remove tags from a setting (unsupported)",
		ArgsUsage: "<key> <key>...",
		Description: `Remove tags from a setting.

Note: the azappconfig SDK cannot write setting tags without clearing them, so
this command reports that tag mutation is unsupported rather than losing data.

EXAMPLES:
   suve azure param untag app/timeout env                    Reports "tags unsupported"`,
		Noun:       "setting",
		UsageError: "usage: suve azure param untag <key> <key> [key]",
		NewTagger:  newTagger,
	})
}
