package secret

import (
	"context"

	"github.com/urfave/cli/v3"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the Secrets Manager provider.Tagger.
//
//declscope:package // untag.go's UntagCommand builds the same Tagger
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return cliinternal.SecretStore(ctx)
}

// TagCommand returns the Secrets Manager tag command.
func TagCommand() *cli.Command {
	return generictag.TagCommand(generictag.Config{
		Usage:     "Add or update tags on a secret",
		ArgsUsage: "<name> <key=value>...",
		Description: `Add or update one or more tags on an existing secret.

Tags are key=value pairs. If a tag key already exists, its value will be updated.
You can specify multiple tags in a single command.

EXAMPLES:
   suve secret tag my-api-key env=prod                   Add single tag
   suve secret tag my-api-key env=prod team=backend      Add multiple tags
   suve secret tag my-api-key env=staging                Update existing tag`,
		Noun:       nounSecret,
		UsageError: "usage: suve secret tag <name> <key=value> [key=value]",
		NewTagger:  newTagger,
	})
}
