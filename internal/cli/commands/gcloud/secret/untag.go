package secret

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/generic"
)

// UntagCommand returns the Google Cloud Secret Manager untag command.
func UntagCommand() *cli.Command {
	return generic.UntagCommand(generic.TagConfig{
		Usage:     `Remove tags from a secret (Google Cloud calls these "labels")`,
		ArgsUsage: "<name> <key>...",
		Description: `Remove one or more tags from an existing secret.

Specify the tag keys to remove. Non-existent keys are silently ignored.

NOTE: Google Cloud Secret Manager natively calls these "labels". suve uses its
cross-provider term "tags" for this key=value metadata everywhere.

EXAMPLES:
   suve gcloud secret untag my-api-key deprecated             Remove single tag
   suve gcloud secret untag my-api-key env team               Remove multiple tags`,
		Noun:       nounSecret,
		UsageError: "usage: suve gcloud secret untag <name> <key> [key]",
		NewTagger:  newTagger,
	})
}
