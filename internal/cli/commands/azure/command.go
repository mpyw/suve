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
		Name:    "azure",
		Aliases: []string{"az"},
		Usage:   "Interact with Azure Key Vault (secret) and App Configuration (param)",
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
		Flags: baseFlags(),
		// Before resolves the shared subscription/resource-group once and stashes
		// them in the context so the generic command presenters (which do not
		// receive *cli.Command) can resolve a store. Resolution is deferred to
		// store construction, so `suve azure --help` still works without them.
		Before: withBase,
		Commands: []*cli.Command{
			secret.Command(),
			param.Command(),
			StageCommand(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// FlatSecretCommand returns the Azure Key Vault (secret) command as a standalone
// top-level command named `name`. Because there is no parent azure group to
// carry them, it folds in the shared --subscription / --resource-group flags and
// base Before hook on top of the secret group's own --vault-name flag/hook. Used
// for the flat `suve secret` alias when Azure is the uniquely active secret
// provider.
func FlatSecretCommand(name string) *cli.Command {
	c := secret.Command()
	c.Name = name

	return foldBase(c)
}

// FlatParamCommand returns the Azure App Configuration (param) command as a
// standalone top-level command named `name`, folding in the base
// subscription/resource-group flags and hook on top of the param group's own
// --store-name flag/hook. Used for the flat `suve param` alias when Azure is the
// uniquely active param provider.
func FlatParamCommand(name string) *cli.Command {
	c := param.Command()
	c.Name = name

	return foldBase(c)
}

// baseFlags returns the shared Azure scope flags (a fresh slice per call).
func baseFlags() []cli.Flag {
	return []cli.Flag{
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
	}
}

// withBase stashes the resolved subscription/resource-group into the context.
func withBase(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	return cliinternal.WithAzureBase(ctx, cmd.String("subscription"), cmd.String("resource-group")), nil
}

// foldBase prepends the base scope flags to c and wraps its Before so the base
// subscription/resource-group are resolved before the command's own hook. It
// turns a subgroup command (which normally relies on the azure parent) into a
// self-contained top-level command.
func foldBase(c *cli.Command) *cli.Command {
	inner := c.Before
	c.Flags = append(baseFlags(), c.Flags...)
	c.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		ctx, err := withBase(ctx, cmd)
		if err != nil {
			return ctx, err
		}

		if inner != nil {
			return inner(ctx, cmd)
		}

		return ctx, nil
	}

	return c
}
