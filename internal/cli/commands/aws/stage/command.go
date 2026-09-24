// Package stage provides the "suve aws stage" command group: the param and
// secret staging subgroups plus the all-service commands (status / diff /
// apply / reset / export / import) that span both AWS services.
//
// command.go is this package's subject: the stage command group it assembles
// from the per-service subgroups in param.go and secret.go. Hence core.
//
//declscope:core
package stage

import (
	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

// providerLabel names AWS in staging prompts and messages.
//
//declscope:package // param.go and secret.go label their stage configs with it
const providerLabel = "AWS"

// GlobalConfig builds the provider-wide stage config for AWS: param + secret
// share one account/region staging scope.
func GlobalConfig() stgcli.GlobalConfig {
	paramCfg, secretCfg := ParamConfig(), SecretConfig()

	return stgcli.GlobalConfig{
		ProviderLabel: providerLabel,
		CommandPath:   "suve aws stage",
		ScopeResolver: awsinternal.StagingScopeResolver,
		Services: []stgcli.GlobalServiceSpec{
			{
				Service:       staging.ServiceParam,
				ParserFactory: paramCfg.ParserFactory,
				Factory:       paramCfg.Factory,
				ScopeResolver: paramCfg.ScopeResolver,
			},
			{
				Service:       staging.ServiceSecret,
				ParserFactory: secretCfg.ParserFactory,
				Factory:       secretCfg.Factory,
				ScopeResolver: secretCfg.ScopeResolver,
			},
		},
	}
}

// Command returns the "suve aws stage" command with its subcommands.
func Command() *cli.Command {
	gcfg := GlobalConfig()

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
			ParamCommand(),
			SecretCommand(),
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
