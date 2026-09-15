package secret

import (
	"github.com/urfave/cli/v3"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
)

// UntagCommand returns the Secrets Manager untag command.
func UntagCommand() *cli.Command {
	return generictag.UntagCommand(generictag.Config{
		Usage:     "Remove tags from a secret",
		ArgsUsage: "<name> <key>...",
		Description: `Remove one or more tags from an existing secret.

Specify the tag keys to remove. Non-existent keys are silently ignored.

EXAMPLES:
   suve secret untag my-api-key deprecated               Remove single tag
   suve secret untag my-api-key env team                 Remove multiple tags`,
		Noun:       nounSecret,
		UsageError: "usage: suve secret untag <name> <key> [key]",
		NewTagger:  newTagger,
	})
}
