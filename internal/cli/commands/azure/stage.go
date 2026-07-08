package azure

import (
	"context"

	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/stage/apply"
	"github.com/mpyw/suve/internal/cli/commands/stage/diff"
	"github.com/mpyw/suve/internal/cli/commands/stage/reset"
	"github.com/mpyw/suve/internal/cli/commands/stage/status"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

// nounSecret is the command / item name used across the Key Vault stage subgroup.
const nounSecret = "secret"

// keyVaultStageConfig is the staging config for Azure Key Vault secrets. The
// ScopeResolver keys on-disk staging state by the resolved vault.
func keyVaultStageConfig() stgcli.CommandConfig {
	return stgcli.CommandConfig{
		CommandName:   nounSecret,
		ItemName:      nounSecret,
		Factory:       cliinternal.AzureKeyVaultSecretStrategyFactory,
		ParserFactory: staging.AzureKeyVaultSecretParserFactory,
		ScopeResolver: cliinternal.AzureKeyVaultStagingScopeResolver,
	}
}

// appConfigStageConfig is the staging config for Azure App Configuration. The
// ScopeResolver keys on-disk staging state by the resolved store.
func appConfigStageConfig() stgcli.CommandConfig {
	return stgcli.CommandConfig{
		CommandName:   "param",
		ItemName:      "setting",
		Factory:       cliinternal.AzureAppConfigParamStrategyFactory,
		ParserFactory: staging.AzureAppConfigParamParserFactory,
		ScopeResolver: cliinternal.AzureAppConfigStagingScopeResolver,
		// App Configuration keys are per-(name, namespace): the --namespace value
		// (resolved into ctx) is recorded on each staged entry, and status/diff/
		// apply resolve a store scoped to each entry's own namespace so one
		// per-store staging file spans every namespace (#431).
		Namespace: cliinternal.AzureAppConfigNamespace,
		StrategyForNamespace: func(ctx context.Context, namespace string) (staging.FullStrategy, error) {
			return cliinternal.AzureAppConfigParamStrategyFactory(
				cliinternal.WithAzureAppConfigNamespace(ctx, namespace),
			)
		},
	}
}

// keyVaultStageSubcommands are the full staging subcommands for Key Vault
// (tags are supported).
func keyVaultStageSubcommands(cfg stgcli.CommandConfig) []*cli.Command {
	return []*cli.Command{
		stgcli.NewAddCommand(cfg),
		stgcli.NewEditCommand(cfg),
		stgcli.NewDeleteCommand(cfg),
		stgcli.NewStatusCommand(cfg),
		stgcli.NewDiffCommand(cfg),
		stgcli.NewApplyCommand(cfg),
		stgcli.NewResetCommand(cfg),
		stgcli.NewTagCommand(cfg),
		stgcli.NewUntagCommand(cfg),
		stgcli.NewStashCommand(cfg),
	}
}

// appConfigStageSubcommands are the staging subcommands for App Configuration.
// tag/untag are included: setting tags are writable via GET-merge-PUT
// (azappconfig/v2).
func appConfigStageSubcommands(cfg stgcli.CommandConfig) []*cli.Command {
	return []*cli.Command{
		stgcli.NewAddCommand(cfg),
		stgcli.NewEditCommand(cfg),
		stgcli.NewDeleteCommand(cfg),
		stgcli.NewStatusCommand(cfg),
		stgcli.NewDiffCommand(cfg),
		stgcli.NewApplyCommand(cfg),
		stgcli.NewResetCommand(cfg),
		stgcli.NewTagCommand(cfg),
		stgcli.NewUntagCommand(cfg),
		stgcli.NewStashCommand(cfg),
	}
}

// keyVaultStageGroup is the "secret" staging subgroup (Key Vault). It owns the
// --vault-name flag and resolves it into the context for the scope resolver.
func keyVaultStageGroup() *cli.Command {
	return &cli.Command{
		Name:    nounSecret,
		Aliases: []string{"kv", "keyvault"},
		Usage:   "Staging operations for Azure Key Vault secrets",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "vault-name",
				Usage:   "Azure Key Vault name (defaults to $AZURE_KEYVAULT_NAME)",
				Sources: cli.EnvVars("AZURE_KEYVAULT_NAME"),
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			return cliinternal.WithAzureVaultName(ctx, cmd.String("vault-name")), nil
		},
		Commands:        keyVaultStageSubcommands(keyVaultStageConfig()),
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// appConfigStageGroup is the "param" staging subgroup (App Configuration). It
// owns the --store-name flag and resolves it into the context.
func appConfigStageGroup() *cli.Command {
	return &cli.Command{
		Name:    "param",
		Aliases: []string{"appconfig", "ac", "appcfg"},
		Usage:   "Staging operations for Azure App Configuration settings",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "store-name",
				Usage:   "Azure App Configuration store name (defaults to $AZURE_APPCONFIG_NAME)",
				Sources: cli.EnvVars("AZURE_APPCONFIG_NAME"),
			},
			&cli.StringFlag{
				Name:    "namespace",
				Aliases: []string{"ns"},
				Usage: "App Configuration namespace to stage under (the label axis; Azure calls " +
					`it a "label"). Staging is per-(store, namespace); a staged op needs one ` +
					"namespace. Empty = the default namespace (defaults to $AZURE_APPCONFIG_NAMESPACE)",
				Sources: cli.EnvVars("AZURE_APPCONFIG_NAMESPACE"),
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			ctx = cliinternal.WithAzureStoreName(ctx, cmd.String("store-name"))
			ctx = cliinternal.WithAzureAppConfigNamespace(ctx, cmd.String("namespace"))

			return ctx, nil
		},
		Commands:        appConfigStageSubcommands(appConfigStageConfig()),
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

const stageDescription = `Stage changes locally before applying to Azure.

Azure staging is per-service, because Key Vault and App Configuration keep
separate staging state:
  - "suve azure stage secret" stages Key Vault secrets (opaque-versioned).
  - "suve azure stage param"  stages App Configuration settings (unversioned,
    last-write-wins; tags are writable via GET-merge-PUT).

Global commands span both services. A service is included only when it is
configured (its resource is named); otherwise it is skipped — an unconfigured
resource can hold no staged state. Set the resource with --store-name /
--vault-name (or the AZURE_APPCONFIG_NAME / AZURE_KEYVAULT_NAME environment
variables):
   status    Show all staged changes (Key Vault and App Configuration)
   diff      Show diff of all staged changes vs Azure
   apply     Apply all staged changes to Azure
   reset     Unstage all changes

EXAMPLES:
   suve azure stage secret add my-secret     Stage a new Key Vault secret
   suve azure stage secret apply             Apply staged Key Vault changes
   suve azure stage param edit my-setting    Edit and stage an App Config setting
   suve azure stage param apply              Apply staged App Config changes
   suve azure stage status                   View all staged changes
   suve azure stage apply                    Apply all staged changes`

// stageGlobalFlags are the resource-name flags the top-level global commands
// need so each service's scope resolver can key its staging state. They mirror
// the per-subgroup flags; the parent Before injects both into the context.
func stageGlobalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "store-name",
			Usage:   "Azure App Configuration store name (defaults to $AZURE_APPCONFIG_NAME)",
			Sources: cli.EnvVars("AZURE_APPCONFIG_NAME"),
		},
		&cli.StringFlag{
			Name:    "vault-name",
			Usage:   "Azure Key Vault name (defaults to $AZURE_KEYVAULT_NAME)",
			Sources: cli.EnvVars("AZURE_KEYVAULT_NAME"),
		},
	}
}

// StageCommand returns the "azure stage" command with the secret (Key Vault) and
// param (App Configuration) staging subgroups plus the provider-wide global
// commands (status / diff / apply / reset) spanning both services.
func StageCommand() *cli.Command {
	gcfg := stgcli.AzureGlobalConfig(appConfigStageConfig(), keyVaultStageConfig())

	return &cli.Command{
		Name:        "stage",
		Aliases:     []string{"stg"},
		Usage:       "Manage staged changes for Azure Key Vault and App Configuration",
		Description: stageDescription,
		Flags:       stageGlobalFlags(),
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// Inject both resource names so the global commands' per-service scope
			// resolvers can key state. The per-service subgroups own the same flags
			// and re-inject in their own Before, so subgroup invocations are
			// unaffected.
			ctx = cliinternal.WithAzureStoreName(ctx, cmd.String("store-name"))
			ctx = cliinternal.WithAzureVaultName(ctx, cmd.String("vault-name"))

			return ctx, nil
		},
		Commands: []*cli.Command{
			keyVaultStageGroup(),
			appConfigStageGroup(),
			status.Command(gcfg),
			diff.Command(gcfg),
			apply.Command(gcfg),
			reset.Command(gcfg),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// FlatStageCommand returns the Azure stage command as a standalone top-level
// command named `name` (e.g. "stage"). The secret/param staging subgroups own
// their --vault-name / --store-name flags and hooks, so it is self-contained.
// Used for the flat `suve stage` alias when Azure is the uniquely active staging
// provider.
func FlatStageCommand(name string) *cli.Command {
	c := StageCommand()
	c.Name = name

	return c
}
