package param

import (
	"context"

	"github.com/urfave/cli/v3"

	generictag "github.com/mpyw/suve/internal/cli/commands/generic/tag"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
)

// newTagger builds the SSM Parameter Store provider.Tagger.
func newTagger(ctx context.Context) (provider.Tagger, error) {
	client, err := cliinternal.NewParamClient(ctx)
	if err != nil {
		return nil, err
	}

	return awsparam.New(client), nil
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

// UntagCommand returns the SSM Parameter Store untag command.
func UntagCommand() *cli.Command {
	return generictag.UntagCommand(generictag.Config{
		Usage:     "Remove tags from a parameter",
		ArgsUsage: "<name> <key>...",
		Description: `Remove one or more tags from an existing parameter.

Specify the tag keys to remove. Non-existent keys are silently ignored.

EXAMPLES:
   suve param untag /app/config deprecated              Remove single tag
   suve param untag /app/config env team                Remove multiple tags`,
		Noun:       "parameter",
		UsageError: "usage: suve param untag <name> <key> [key]",
		NewTagger:  newTagger,
	})
}
