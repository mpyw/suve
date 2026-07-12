// Package param provides the param stage subcommand for staging operations.
package param

import (
	"errors"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/aws/param/paramtype"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

//nolint:gochecknoglobals // package-level config for command factory
var config = stgcli.CommandConfig{
	CommandName:      "param",
	ItemName:         "parameter",
	Factory:          cliinternal.AWSParamStrategyFactory,
	ParserFactory:    staging.AWSParamParserFactory,
	HasDescription:   true,
	ValueTypeFlags:   valueTypeFlags(),
	ValueTypeFromCmd: resolveValueType,
}

// valueTypeFlags returns the SSM Parameter Store type flags for stage add/edit,
// matching the immediate `param create`/`param update` commands. --type carries
// no default so an unset flag stays "not specified" (create then applies String,
// edit preserves the existing type).
func valueTypeFlags() []cli.Flag {
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

// resolveValueType maps the --type/--secure flags to a domain.ValueType, using
// the same mutual-exclusion and validation as immediate `param create`. It
// returns an empty value type when neither flag is set, meaning "not specified".
func resolveValueType(cmd *cli.Command) (domain.ValueType, error) {
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

// Config returns the AWS SSM Parameter Store staging command config. It is used
// by the global (all-service) stage commands to build their provider config.
func Config() stgcli.CommandConfig {
	return config
}

// Command returns the param stage command with all staging subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:    "param",
		Aliases: []string{"ssm", "ps"},
		Usage:   "Staging operations for SSM Parameter Store parameters",
		Description: `Stage changes locally before applying to AWS.

Use 'suve stage param add' to create and stage a new parameter.
Use 'suve stage param edit' to edit and stage an existing parameter.
Use 'suve stage param delete' to stage a parameter for deletion.
Use 'suve stage param status' to view staged parameter changes.
Use 'suve stage param diff' to see differences between staged and AWS values.
Use 'suve stage param apply' to apply staged parameter changes to AWS.
Use 'suve stage param reset' to unstage or restore from a version.`,
		Commands: []*cli.Command{
			stgcli.NewAddCommand(config),
			stgcli.NewEditCommand(config),
			stgcli.NewDeleteCommand(config),
			stgcli.NewStatusCommand(config),
			stgcli.NewDiffCommand(config),
			stgcli.NewApplyCommand(config),
			stgcli.NewResetCommand(config),
			stgcli.NewTagCommand(config),
			stgcli.NewUntagCommand(config),
			stgcli.NewExportCommand(config),
			stgcli.NewImportCommand(config),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
