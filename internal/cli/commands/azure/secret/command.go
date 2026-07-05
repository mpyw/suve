// Package secret provides CLI commands for Azure Key Vault secrets, exposed as
// the "suve azure secret <op>" command group.
//
// Key Vault secrets are versioned by opaque ids (there are no staging labels),
// so this group exposes the read/write/tag commands (show, log, list, diff,
// create, update, delete, tag, untag) reusing the generic command scaffolding
// via Azure-specific presenters and the shared internal/usecase/azure use cases.
package secret

import (
	"context"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/version/azurekvversion"
)

// Command returns the "azure secret" subcommand group.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "secret",
		Usage: "Interact with Azure Key Vault secrets",
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
		// Before merges the resolved vault name onto the base scope (subscription
		// / resource group) set by the parent azure command. Resolution is
		// deferred to store construction, so `suve azure secret --help` works
		// without a vault.
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			return cliinternal.WithAzureVaultName(ctx, cmd.String("vault-name")), nil
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

// specSuffix reconstructs the version-spec suffix (the part after the name) from
// a parsed Key Vault spec, so that name+suffix re-parses to an equivalent spec.
// It is handed to provider.Reader.Resolve via the use cases.
//
// Examples: {ID:"abc"} -> "#abc"; {Shift:2} -> "~2"; {} -> "" (current).
func specSuffix(spec *azurekvversion.Spec) string {
	var b strings.Builder

	if spec.Absolute.ID != nil {
		b.WriteString("#")
		b.WriteString(*spec.Absolute.ID)
	}

	if spec.Shift > 0 {
		b.WriteString("~")
		b.WriteString(strconv.Itoa(spec.Shift))
	}

	return b.String()
}
