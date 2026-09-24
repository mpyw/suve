package stage

import (
	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

// nounSecret is the command / item name used across the secret stage command.
const nounSecret = "secret"

//nolint:gochecknoglobals // package-level config for command factory
var secretConfig = stgcli.CommandConfig{
	CommandName:    nounSecret,
	ItemName:       nounSecret,
	ProviderLabel:  providerLabel,
	CommandPath:    "suve aws stage secret",
	Factory:        cliinternal.StrategyFactory(provider.ProviderAWS, provider.KindSecret, awsinternal.SecretStore),
	ParserFactory:  cliinternal.ParserFactory(provider.ProviderAWS, provider.KindSecret),
	ScopeResolver:  awsinternal.StagingScopeResolver,
	HasDescription: true,
}

// SecretConfig returns the AWS Secrets Manager staging command config. It is used by
// the global (all-service) stage commands to build their provider config.
func SecretConfig() stgcli.CommandConfig {
	return secretConfig
}

// SecretCommand returns the secret stage command with all staging subcommands.
func SecretCommand() *cli.Command {
	return &cli.Command{
		Name:    nounSecret,
		Aliases: []string{"sm", "secretsmanager"},
		Usage:   "Staging operations for Secrets Manager",
		Description: `Stage changes locally before applying to AWS.

Use 'suve aws stage secret add' to create and stage a new secret.
Use 'suve aws stage secret edit' to edit and stage an existing secret.
Use 'suve aws stage secret delete' to stage a secret for deletion.
Use 'suve aws stage secret status' to view staged secret changes.
Use 'suve aws stage secret diff' to see differences between staged and AWS values.
Use 'suve aws stage secret apply' to apply staged secret changes to AWS.
Use 'suve aws stage secret reset' to unstage or restore from a version.`,
		Commands: []*cli.Command{
			stgcli.NewAddCommand(secretConfig),
			stgcli.NewEditCommand(secretConfig),
			stgcli.NewDeleteCommand(secretConfig),
			stgcli.NewStatusCommand(secretConfig),
			stgcli.NewDiffCommand(secretConfig),
			stgcli.NewApplyCommand(secretConfig),
			stgcli.NewResetCommand(secretConfig),
			stgcli.NewTagCommand(secretConfig),
			stgcli.NewUntagCommand(secretConfig),
			stgcli.NewExportCommand(secretConfig),
			stgcli.NewImportCommand(secretConfig),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
