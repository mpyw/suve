package param

import (
	"context"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/provider"
)

// newTagger builds the SSM Parameter Store provider.Tagger.
//
//declscope:package // untag.go's UntagCommand builds the same Tagger
func newTagger(ctx context.Context) (provider.Tagger, error) {
	return awsinternal.ParamStore(ctx)
}

// TagCommand returns the SSM Parameter Store tag command.
func TagCommand() *cli.Command {
	return generic.TagCommand(generic.TagConfig{
		Usage:     "Add or update tags on a parameter",
		ArgsUsage: "<name> <key=value>...",
		Description: `Add or update one or more tags on an existing parameter.

Tags are key=value pairs. If a tag key already exists, its value will be updated.
You can specify multiple tags in a single command.

EXAMPLES:
   suve aws param tag /app/config env=prod                     Add single tag
   suve aws param tag /app/config env=prod team=backend        Add multiple tags
   suve aws param tag /app/config env=staging                  Update existing tag`,
		Noun:       "parameter",
		UsageError: "usage: suve aws param tag <name> <key=value> [key=value]",
		NewTagger:  newTagger,
	})
}
