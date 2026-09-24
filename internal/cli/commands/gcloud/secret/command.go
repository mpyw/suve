// Package secret provides CLI commands for Google Cloud Secret Manager, exposed
// as the "suve gcloud secret <op>" command group.
//
// Google Cloud secrets are integer-versioned and have no staging labels, so this
// group exposes the read/write/tag commands (show, log, list, diff, create,
// update, delete, tag, untag) reusing the generic command scaffolding via
// Google Cloud-specific presenters and the provider-neutral
// internal/usecase/secret use cases. The --project flag and the Before hook that
// resolves it belong to the parent gcloud group.
//
// command.go is this package's subject: the gcloud secret command group it
// assembles, together with the vocabulary its sibling files share.
// Hence core.
//
//declscope:core
package secret

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// nounSecret is the command name / noun used across the Google Cloud secret commands.
//
//declscope:package // tag.go and untag.go name their commands with it
const nounSecret = "secret"

// Command returns the "gcloud secret" subcommand group.
func Command() *cli.Command {
	return &cli.Command{
		Name:    nounSecret,
		Aliases: []string{"secrets", "sm"},
		Usage:   "Interact with Google Cloud Secret Manager secrets",
		Commands: []*cli.Command{
			ShowCommand(),
			LogCommand(),
			DiffCommand(),
			ListCommand(),
			CreateCommand(),
			UpdateCommand(),
			DeleteCommand(),
			TagCommand(),
			UntagCommand(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
