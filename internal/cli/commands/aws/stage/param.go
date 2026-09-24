package stage

import (
	"errors"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

//nolint:gochecknoglobals // package-level config for command factory
var paramConfig = stgcli.CommandConfig{
	CommandName:      "param",
	ItemName:         "parameter",
	ProviderLabel:    providerLabel,
	CommandPath:      "suve aws stage param",
	Factory:          cliinternal.StrategyFactory(provider.ProviderAWS, provider.KindParam, awsinternal.ParamStore),
	ParserFactory:    cliinternal.ParserFactory(provider.ProviderAWS, provider.KindParam),
	ScopeResolver:    awsinternal.StagingScopeResolver,
	HasDescription:   true,
	ValueTypeFlags:   paramValueTypeFlags(),
	ValueTypeFromCmd: resolveParamValueType,
}

// paramValueTypeFlags returns the SSM Parameter Store type flags for stage add/edit,
// matching the immediate `param create`/`param update` commands. --type carries
// no default so an unset flag stays "not specified" (create then applies String,
// edit preserves the existing type).
func paramValueTypeFlags() []cli.Flag {
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

// resolveParamValueType maps the --type/--secure flags to a domain.ValueType, using
// the same mutual-exclusion and validation as immediate `param create`. It
// returns an empty value type when neither flag is set, meaning "not specified".
func resolveParamValueType(cmd *cli.Command) (domain.ValueType, error) {
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

// ParamConfig returns the AWS SSM Parameter Store staging command config. It is used
// by the global (all-service) stage commands to build their provider config.
func ParamConfig() stgcli.CommandConfig {
	return paramConfig
}

// ParamCommand returns the param stage command with all staging subcommands.
func ParamCommand() *cli.Command {
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
		Commands: []*cli.Command{
			stgcli.NewAddCommand(paramConfig),
			stgcli.NewEditCommand(paramConfig),
			stgcli.NewDeleteCommand(paramConfig),
			stgcli.NewStatusCommand(paramConfig),
			stgcli.NewDiffCommand(paramConfig),
			stgcli.NewApplyCommand(paramConfig),
			stgcli.NewResetCommand(paramConfig),
			stgcli.NewTagCommand(paramConfig),
			stgcli.NewUntagCommand(paramConfig),
			stgcli.NewExportCommand(paramConfig),
			stgcli.NewImportCommand(paramConfig),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
