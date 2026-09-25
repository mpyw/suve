// Package param provides CLI commands for AWS SSM Parameter Store.
// command.go is this package's subject: the aws param command group it
// assembles, together with the vocabulary its sibling files share. Hence core.
//
//declscope:core
package param

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/capability"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
)

// Command returns the param command with all subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "param",
		Aliases: []string{"ssm", "ps"},
		Usage:   "Interact with AWS Systems Manager Parameter Store",
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

// itemNoun names one Parameter Store item ("parameter") in the shared use
// cases' error messages, from the capability matrix.
//
//declscope:package // create, update and delete pass it to their use cases
func itemNoun() string {
	sc, _ := capability.Service(provider.ProviderAWS, string(provider.KindParam))

	return sc.ItemNoun
}
