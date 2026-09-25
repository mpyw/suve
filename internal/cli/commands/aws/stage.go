package aws

import (
	"errors"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

// stageNounSecret is the command / item name used across the Secrets Manager
// stage subgroup.
const stageNounSecret = "secret"

// stageProviderLabel names AWS in staging prompts and messages.
const stageProviderLabel = "AWS"

// parameterStoreStageConfig is the staging config for SSM Parameter Store. It
// adds the --type / --secure value-type flags to stage add/edit.
func parameterStoreStageConfig() stgcli.CommandConfig {
	return stgcli.CommandConfig{
		CommandName:      "param",
		ItemName:         "parameter",
		ProviderLabel:    stageProviderLabel,
		CommandPath:      "suve aws stage param",
		Factory:          cliinternal.StrategyFactory(provider.ProviderAWS, provider.KindParam, awsinternal.ParamStore),
		ParserFactory:    cliinternal.ParserFactory(provider.ProviderAWS, provider.KindParam),
		ScopeResolver:    awsinternal.StagingScopeResolver,
		HasDescription:   true,
		ValueTypeFlags:   stageParamValueTypeFlags(),
		ValueTypeFromCmd: resolveStageParamValueType,
	}
}

// secretsManagerStageConfig is the staging config for Secrets Manager.
func secretsManagerStageConfig() stgcli.CommandConfig {
	return stgcli.CommandConfig{
		CommandName:    stageNounSecret,
		ItemName:       stageNounSecret,
		ProviderLabel:  stageProviderLabel,
		CommandPath:    "suve aws stage secret",
		Factory:        cliinternal.StrategyFactory(provider.ProviderAWS, provider.KindSecret, awsinternal.SecretStore),
		ParserFactory:  cliinternal.ParserFactory(provider.ProviderAWS, provider.KindSecret),
		ScopeResolver:  awsinternal.StagingScopeResolver,
		HasDescription: true,
	}
}

// stageParamValueTypeFlags returns the SSM Parameter Store type flags for stage add/edit,
// matching the immediate `param create`/`param update` commands. --type carries
// no default so an unset flag stays "not specified" (create then applies String,
// edit preserves the existing type).
func stageParamValueTypeFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "type",
			Usage: "Parameter type (String, StringList, SecureString)",
		},
		&cli.BoolFlag{
			Name:  "secure",
			Usage: "Shorthand for --type SecureString",
		},
	}
}

// resolveStageParamValueType maps the --type/--secure flags to a domain.ValueType, using
// the same mutual-exclusion and validation as immediate `param create`. It
// returns an empty value type when neither flag is set, meaning "not specified".
func resolveStageParamValueType(cmd *cli.Command) (domain.ValueType, error) {
	secure := cmd.Bool("secure")
	typeSet := cmd.IsSet("type")

	if secure && typeSet {
		return "", errors.New("cannot use --secure with --type; use one or the other")
	}

	switch {
	case secure:
		return domain.ValueTypeSecret, nil
	case typeSet:
		paramType := cmd.String("type")
		if err := paramtype.Validate(paramType); err != nil {
			return "", err
		}

		return paramtype.Parse(paramType), nil
	default:
		return "", nil
	}
}

// stageSubcommands are the full staging subcommands for one AWS service.
func stageSubcommands(cfg stgcli.CommandConfig) []*cli.Command {
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
		stgcli.NewExportCommand(cfg),
		stgcli.NewImportCommand(cfg),
	}
}

// StageParamCommand returns the "param" staging subgroup (SSM Parameter Store).
func StageParamCommand() *cli.Command {
	return &cli.Command{
		Name:    "param",
		Aliases: []string{"ssm", "ps"},
		Usage:   "Staging operations for SSM Parameter Store parameters",
		Description: `Stage changes locally before applying to AWS.

Use 'suve aws stage param add' to create and stage a new parameter.
Use 'suve aws stage param edit' to edit and stage an existing parameter.
Use 'suve aws stage param delete' to stage a parameter for deletion.
Use 'suve aws stage param status' to view staged parameter changes.
Use 'suve aws stage param diff' to see differences between staged and AWS values.
Use 'suve aws stage param apply' to apply staged parameter changes to AWS.
Use 'suve aws stage param reset' to unstage or restore from a version.`,
		Commands:        stageSubcommands(parameterStoreStageConfig()),
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// StageSecretCommand returns the "secret" staging subgroup (Secrets Manager).
func StageSecretCommand() *cli.Command {
	return &cli.Command{
		Name:    stageNounSecret,
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
		Commands:        stageSubcommands(secretsManagerStageConfig()),
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// StageGlobalConfig builds the provider-wide stage config for AWS: param +
// secret share one account/region staging scope, so the all-service commands
// include export and import.
func StageGlobalConfig() stgcli.GlobalConfig {
	paramCfg, secretCfg := parameterStoreStageConfig(), secretsManagerStageConfig()
	// Both services resolve the same STS identity: resolve it once per command.
	shared := stgcli.SharedScopeResolver(awsinternal.StagingScopeResolver)

	return stgcli.GlobalConfig{
		ProviderLabel: stageProviderLabel,
		CommandPath:   "suve aws stage",
		ScopeResolver: shared,
		Services: []stgcli.GlobalServiceSpec{
			{
				Service:       staging.ServiceParam,
				ParserFactory: paramCfg.ParserFactory,
				Factory:       paramCfg.Factory,
				ScopeResolver: shared,
			},
			{
				Service:       staging.ServiceSecret,
				ParserFactory: secretCfg.ParserFactory,
				Factory:       secretCfg.Factory,
				ScopeResolver: shared,
			},
		},
	}
}

// StageCommand returns the "aws stage" command with the param and secret
// staging subgroups plus the provider-wide global commands (status / diff /
// apply / reset / export / import) spanning both services.
func StageCommand() *cli.Command {
	gcfg := StageGlobalConfig()

	return &cli.Command{
		Name:    "stage",
		Aliases: []string{"stg"},
		Usage:   "Manage staged changes for AWS Parameter Store and Secrets Manager",
		Description: `Stage changes locally before applying to AWS.

Use 'suve aws stage param' for SSM Parameter Store operations.
Use 'suve aws stage secret' for Secrets Manager operations.

Global commands operate on all staged changes:
   status    Show all staged changes (SSM Parameter Store and Secrets Manager)
   diff      Show diff of all staged changes vs AWS
   apply     Apply all staged changes to AWS
   reset     Unstage all changes
   export    Export staged changes to a directory (one file per service)
   import    Import staged changes from a directory

EXAMPLES:
   suve aws stage param add /my/param       Stage a new SSM Parameter Store parameter
   suve aws stage secret edit my-secret     Edit and stage a secret
   suve aws stage status                    View all staged changes
   suve aws stage apply                     Apply all staged changes
   suve aws stage export ./backup           Export staged changes to a directory
   suve aws stage import ./backup           Import staged changes from a directory`,
		Commands: []*cli.Command{
			StageParamCommand(),
			StageSecretCommand(),
			stgcli.NewGlobalStatusCommand(gcfg),
			stgcli.NewGlobalDiffCommand(gcfg),
			stgcli.NewGlobalApplyCommand(gcfg),
			stgcli.NewGlobalResetCommand(gcfg),
			stgcli.NewGlobalExportCommand(gcfg),
			stgcli.NewGlobalImportCommand(gcfg),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// FlatStageCommand returns the AWS stage command as a standalone top-level
// command named `name` (e.g. "stage"), carrying the per-service subgroups and
// the all-service commands. Used for the flat `suve stage` alias when AWS is
// the uniquely active staging provider.
func FlatStageCommand(name string) *cli.Command {
	c := StageCommand()
	c.Name = name

	return c
}
