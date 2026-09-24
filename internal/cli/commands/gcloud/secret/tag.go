package secret

import (
	"context"

	"github.com/urfave/cli/v3"

	gcloudinternal "github.com/mpyw/suve/internal/cli/commands/gcloud/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the Google Cloud Secret Manager provider.Tagger.
//
//declscope:package // untag.go's UntagCommand builds the same Tagger
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return gcloudinternal.SecretStore(ctx)
}

// TagCommand returns the Google Cloud Secret Manager tag command.
func TagCommand() *cli.Command {
	return generic.TagCommand(generic.TagConfig{
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
