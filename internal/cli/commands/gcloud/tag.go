package gcloud

import (
	"context"

	"github.com/urfave/cli/v3"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the Google Cloud Secret Manager provider.Tagger.
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return cliinternal.GoogleCloudSecretStore(ctx)
}

// TagCommand returns the Google Cloud Secret Manager tag command.
func TagCommand() *cli.Command {
	return generictag.TagCommand(generictag.Config{
		Usage:     `Add or update tags on a secret (Google Cloud calls these "labels")`,
		ArgsUsage: "<name> <key=value>...",
		Description: `Add or update one or more tags on an existing secret.

Tags are key=value pairs. If a tag key already exists, its value is updated.
You can specify multiple tags in a single command.

NOTE: Google Cloud Secret Manager natively calls these "labels". suve uses its
cross-provider term "tags" for this key=value metadata everywhere.

EXAMPLES:
   suve gcloud secret tag my-api-key env=prod                 Add single tag
   suve gcloud secret tag my-api-key env=prod team=backend    Add multiple tags`,
		Noun:       nounSecret,
		UsageError: "usage: suve gcloud secret tag <name> <key=value> [key=value]",
		NewTagger:  newTagger,
	})
}

// UntagCommand returns the Google Cloud Secret Manager untag command.
func UntagCommand() *cli.Command {
	return generictag.UntagCommand(generictag.Config{
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
