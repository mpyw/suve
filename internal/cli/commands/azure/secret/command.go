// Package secret provides CLI commands for Azure Key Vault secrets, exposed as
// the "suve azure secret <op>" command group.
//
// Key Vault secrets are versioned by opaque ids (there are no staging labels),
// so this group exposes the read/write/tag commands (show, log, list, diff,
// create, update, delete, tag, untag) reusing the generic command scaffolding
// via Azure-specific presenters and the provider-neutral internal/usecase/secret
// use cases.
//
// command.go is this package's subject: the azure secret command group it
// assembles, together with the vocabulary its sibling files share.
// Hence core.
//
//declscope:core
package secret

import (
	"context"

	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// nounSecret is the command name / noun used across the Key Vault secret commands.
//
//declscope:package // tag.go and untag.go name their commands with it
const nounSecret = "secret"

// argsUsageName is the shared ArgsUsage for single-secret commands.
//
//declscope:package // delete, restore and log use it for a single-secret command's ArgsUsage
const argsUsageName = "<name>"

// Command returns the "azure secret" subcommand group.
func Command() *cli.Command {
	return &cli.Command{
		Name:    nounSecret,
		Aliases: []string{"kv", "keyvault"},
		Usage:   "Interact with Azure Key Vault secrets",
		Description: `Interact with Azure Key Vault secrets.

Key Vault secrets are versioned by opaque ids (e.g. a 32-character hex string)
and have no staging labels. Set the vault with --vault-name or the
AZURE_KEYVAULT_NAME environment variable.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "vault-name",
				Usage:   "Azure Key Vault name (defaults to $AZURE_KEYVAULT_NAME)",
				Sources: cli.EnvVars("AZURE_KEYVAULT_NAME"),
			},
		},
		// Before stashes the resolved vault name in the context. Resolution is
		// deferred to store construction, so `suve azure secret --help` works
		// without a vault.
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			return azureinternal.WithVaultName(ctx, cmd.String("vault-name")), nil
		},
		Commands: []*cli.Command{
			ShowCommand(),
			LogCommand(),
			DiffCommand(),
			ListCommand(),
			CreateCommand(),
			UpdateCommand(),
			DeleteCommand(),
			RestoreCommand(),
			TagCommand(),
			UntagCommand(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
