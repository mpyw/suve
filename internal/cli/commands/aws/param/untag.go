package param

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/generic"
)

// UntagCommand returns the SSM Parameter Store untag command.
func UntagCommand() *cli.Command {
	return generic.UntagCommand(generic.TagConfig{
		Usage:     "Remove tags from a parameter",
		ArgsUsage: "<name> <key>...",
		Description: `Remove one or more tags from an existing parameter.

Specify the tag keys to remove. Non-existent keys are silently ignored.

EXAMPLES:
   suve aws param untag /app/config deprecated              Remove single tag
   suve aws param untag /app/config env team                Remove multiple tags`,
		Noun:       "parameter",
		UsageError: "usage: suve aws param untag <name> <key> [key]",
		NewTagger:  newTagger,
	})
}
