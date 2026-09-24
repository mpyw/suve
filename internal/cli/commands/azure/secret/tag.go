package secret

import (
	"context"

	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the Azure Key Vault provider.Tagger.
//
//declscope:package // untag.go's UntagCommand builds the same Tagger
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return azureinternal.KeyVaultStore(ctx)
}

// TagCommand returns the Azure Key Vault tag command.
func TagCommand() *cli.Command {
	return generic.TagCommand(generic.TagConfig{
		Usage:     "Add or update tags on a secret",
		ArgsUsage: "<name> <key=value>...",
		Description: `Add or update one or more tags on an existing secret.

Tags are key=value pairs. If a tag key already exists, its value is updated.
You can specify multiple tags in a single command.

EXAMPLES:
   suve azure secret tag my-api-key env=prod                 Add single tag
   suve azure secret tag my-api-key env=prod team=backend    Add multiple tags`,
		Noun:       nounSecret,
		UsageError: "usage: suve azure secret tag <name> <key=value> [key=value]",
		NewTagger:  newTagger,
	})
}
