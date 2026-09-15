package param

import (
	"context"

	"github.com/urfave/cli/v3"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the SSM Parameter Store provider.Tagger.
//
//declscope:package // untag.go's UntagCommand builds the same Tagger
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return cliinternal.ParamStore(ctx)
}

// TagCommand returns the SSM Parameter Store tag command.
func TagCommand() *cli.Command {
	return generictag.TagCommand(generictag.Config{
		Usage:     "Add or update tags on a parameter",
		ArgsUsage: "<name> <key=value>...",
		Description: `Add or update one or more tags on an existing parameter.

Tags are key=value pairs. If a tag key already exists, its value will be updated.
You can specify multiple tags in a single command.

EXAMPLES:
   suve param tag /app/config env=prod                     Add single tag
   suve param tag /app/config env=prod team=backend        Add multiple tags
   suve param tag /app/config env=staging                  Update existing tag`,
		Noun:       "parameter",
		UsageError: "usage: suve param tag <name> <key=value> [key=value]",
		NewTagger:  newTagger,
	})
}
