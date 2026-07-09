// Package azure provides CLI commands for Microsoft Azure, exposed as the
// "suve azure secret <op>" (Key Vault) and "suve azure param <op>" (App
// Configuration) command groups.
//
// Azure splits secrets and parameters across two services:
//
//   - Key Vault (secret): opaque-id-versioned, no staging labels.
//   - App Configuration (param): UNVERSIONED — the abstraction's acid test.
//
// Both groups reuse the same generic command scaffolding as the AWS/GoogleCloud commands
// via Azure-specific presenters and use cases. Each subgroup owns its address
// flag (--vault-name / --store-name); the target resource's globally-unique name
// is all that is needed to reach it.
package azure

import (
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/azure/param"
	"github.com/mpyw/suve/internal/cli/commands/azure/secret"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// Command returns the azure command with the secret (Key Vault) and param (App
// Configuration) subcommand groups.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "azure",
		Aliases: []string{"az"},
		Usage:   "Interact with Azure Key Vault (secret) and App Configuration (param)",
		Description: `Interact with Microsoft Azure secret and parameter stores.

Azure splits the two services:
  - "suve azure secret" targets Key Vault (opaque-id-versioned, no labels).
  - "suve azure param"   targets App Configuration (UNVERSIONED).

App Configuration has no version history: #, ~, and : are valid key characters
(the whole argument is the literal key name, not a version specifier), and "log"
reports that history is unsupported.

Authentication uses the DefaultAzureCredential chain (environment, managed
identity, Azure CLI via 'az login', ...). The target resource is addressed by
its globally-unique name (--vault-name / AZURE_KEYVAULT_NAME for Key Vault,
--store-name / AZURE_APPCONFIG_NAME for App Configuration); no subscription or
resource group is required.`,
		Commands: []*cli.Command{
			secret.Command(),
			param.Command(),
			StageCommand(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// FlatSecretCommand returns the Azure Key Vault (secret) command as a standalone
// top-level command named `name`. The secret group already owns its --vault-name
// flag and Before hook, so it is self-contained. Used for the flat `suve secret`
// alias when Azure is the uniquely active secret provider.
func FlatSecretCommand(name string) *cli.Command {
	c := secret.Command()
	c.Name = name

	return c
}

// FlatParamCommand returns the Azure App Configuration (param) command as a
// standalone top-level command named `name`. The param group already owns its
// --store-name flag and Before hook, so it is self-contained. Used for the flat
// `suve param` alias when Azure is the uniquely active param provider.
func FlatParamCommand(name string) *cli.Command {
	c := param.Command()
	c.Name = name

	return c
}
