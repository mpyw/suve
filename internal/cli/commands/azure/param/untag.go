package param

import (
	"github.com/urfave/cli/v3"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
)

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
