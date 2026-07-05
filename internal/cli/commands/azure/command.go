// Package azure provides CLI commands for Microsoft Azure, exposed as the
// "suve azure secret <op>" (Key Vault) and "suve azure param <op>" (App
// Configuration) command groups.
//
// Azure splits secrets and parameters across two services:
//
//   - Key Vault (secret): opaque-id-versioned, no staging labels.
//   - App Configuration (param): UNVERSIONED — the abstraction's acid test.
//
// Both groups reuse the same generic command scaffolding as the AWS/GCP commands
// via Azure-specific presenters and use cases. The top-level azure command owns
// the shared --subscription / --resource-group flags; each subgroup owns its
// address flag (--vault-name / --store-name).
package azure

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/azure/param"
	"github.com/mpyw/suve/internal/cli/commands/azure/secret"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// Command returns the azure command with the secret (Key Vault) and param (App
// Configuration) subcommand groups.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "azure",
		Usage: "Interact with Azure Key Vault (secret) and App Configuration (param)",
		Description: `Interact with Microsoft Azure secret and parameter stores.

Azure splits the two services:
  - "suve azure secret" targets Key Vault (opaque-id-versioned, no labels).
  - "suve azure param"   targets App Configuration (UNVERSIONED).

App Configuration has no version history: version specifiers (#VERSION, ~SHIFT,
:LABEL) are rejected with a clear error, and "log" reports that history is
unsupported.

Set the subscription and resource group with --subscription/--resource-group or
the AZURE_SUBSCRIPTION_ID/AZURE_RESOURCE_GROUP environment variables.
Authentication uses the DefaultAzureCredential chain (environment, managed
identity, Azure CLI, ...).`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "subscription",
				Usage:   "Azure subscription id (defaults to $AZURE_SUBSCRIPTION_ID)",
				Sources: cli.EnvVars("AZURE_SUBSCRIPTION_ID"),
				// Persistent (default): readable by every azure subcommand.
			},
			&cli.StringFlag{
				Name:    "resource-group",
				Usage:   "Azure resource group (defaults to $AZURE_RESOURCE_GROUP)",
				Sources: cli.EnvVars("AZURE_RESOURCE_GROUP"),
			},
		},
		// Before resolves the shared subscription/resource-group once and stashes
		// them in the context so the generic command presenters (which do not
		// receive *cli.Command) can resolve a store. Resolution is deferred to
		// store construction, so `suve azure --help` still works without them.
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			return cliinternal.WithAzureBase(ctx, cmd.String("subscription"), cmd.String("resource-group")), nil
		},
		Commands: []*cli.Command{
			secret.Command(),
			param.Command(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
