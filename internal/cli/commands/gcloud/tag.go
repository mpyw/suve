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
		Usage:     "Add or update labels on a secret",
		ArgsUsage: "<name> <key=value>...",
		Description: `Add or update one or more labels on an existing secret.

Labels are key=value pairs. If a label key already exists, its value is updated.
You can specify multiple labels in a single command.

EXAMPLES:
   suve gcloud secret tag my-api-key env=prod                 Add single label
   suve gcloud secret tag my-api-key env=prod team=backend    Add multiple labels`,
		Noun:       "secret",
		UsageError: "usage: suve gcloud secret tag <name> <key=value> [key=value]",
		NewTagger:  newTagger,
	})
}

// UntagCommand returns the Google Cloud Secret Manager untag command.
func UntagCommand() *cli.Command {
	return generictag.UntagCommand(generictag.Config{
		Usage:     "Remove labels from a secret",
		ArgsUsage: "<name> <key>...",
		Description: `Remove one or more labels from an existing secret.

Specify the label keys to remove. Non-existent keys are silently ignored.

EXAMPLES:
   suve gcloud secret untag my-api-key deprecated             Remove single label
   suve gcloud secret untag my-api-key env team               Remove multiple labels`,
		Noun:       "secret",
		UsageError: "usage: suve gcloud secret untag <name> <key> [key]",
		NewTagger:  newTagger,
	})
}
