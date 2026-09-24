package secret

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/generic"
)

// UntagCommand returns the Secrets Manager untag command.
func UntagCommand() *cli.Command {
	return generic.UntagCommand(generic.TagConfig{
		Usage:     "Remove tags from a secret",
		ArgsUsage: "<name> <key>...",
		Description: `Remove one or more tags from an existing secret.

Specify the tag keys to remove. Non-existent keys are silently ignored.

EXAMPLES:
   suve aws secret untag my-api-key deprecated               Remove single tag
   suve aws secret untag my-api-key env team                 Remove multiple tags`,
		Noun:       nounSecret,
		UsageError: "usage: suve aws secret untag <name> <key> [key]",
		NewTagger:  newTagger,
	})
}
