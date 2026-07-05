// Package param provides CLI commands for Azure App Configuration, exposed as
// the "suve azure param <op>" command group.
//
// Azure App Configuration is UNVERSIONED — the abstraction's acid test. Version
// specifiers (#VERSION, ~SHIFT, :LABEL) are rejected at parse time with a clear
// error, and "log" reports that version history is unsupported instead of
// crashing. The group otherwise reuses the generic command scaffolding (show,
// list, diff, create, update, delete, tag, untag) via App Configuration
// presenters and the shared internal/usecase/azure use cases.
package param

import (
	"context"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// Command returns the "azure param" subcommand group.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "param",
		Usage: "Interact with Azure App Configuration key-values",
		Description: `Interact with Azure App Configuration key-values.

App Configuration is UNVERSIONED: each key (with the default label) holds a
single current value with no history. Version specifiers (#VERSION, ~SHIFT,
:LABEL) are rejected with a clear error, and "log" reports that history is
unsupported.

Set the store with --store-name or the AZURE_APPCONFIG_NAME environment
variable.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "store-name",
				Usage:   "Azure App Configuration store name (defaults to $AZURE_APPCONFIG_NAME)",
				Sources: cli.EnvVars("AZURE_APPCONFIG_NAME"),
			},
		},
		// Before merges the resolved store name onto the base scope (subscription
		// / resource group) set by the parent azure command. Resolution is
		// deferred to store construction, so `suve azure param --help` works
		// without a store.
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			return cliinternal.WithAzureStoreName(ctx, cmd.String("store-name")), nil
		},
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
