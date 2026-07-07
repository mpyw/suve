package gcloud

import (
	"github.com/urfave/cli/v3"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

// gcloudStageConfig is the staging command config for Google Cloud Secret Manager.
// Because Google Cloud is secret-only, the single config drives the whole
// `gcloud stage` group directly (no param/secret split). The ScopeResolver keys
// on-disk staging state by the resolved project.
func gcloudStageConfig() stgcli.CommandConfig {
	return stgcli.CommandConfig{
		CommandName:   nounSecret,
		ItemName:      nounSecret,
		Factory:       cliinternal.GoogleCloudSecretStrategyFactory,
		ParserFactory: staging.GoogleCloudSecretParserFactory,
		ScopeResolver: cliinternal.GoogleCloudStagingScopeResolver,
	}
}

// stageDescription is shared by the grouped and flat forms of the command.
const stageDescription = `Stage changes locally before applying to Google Cloud Secret Manager.

Google Cloud is secret-only, so 'suve gcloud stage' operates on secrets directly:
   add       Stage a new secret for creation
   edit      Edit and stage an existing secret
   delete    Stage a secret for deletion
   status    Show staged changes
   diff      Show diff of staged changes vs Google Cloud
   apply     Apply staged changes to Google Cloud
   reset     Unstage changes
   tag/untag Stage label changes
   stash     Save/restore staged changes to/from file

EXAMPLES:
   suve gcloud stage add my-secret       Stage a new secret
   suve gcloud stage edit my-secret      Edit and stage a secret
   suve gcloud stage status              View staged changes
   suve gcloud stage apply               Apply staged changes`

// stageSubcommands builds the staging subcommands for the given config.
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
		stgcli.NewStashCommand(cfg),
	}
}

// StageCommand returns the "gcloud stage" subcommand group.
func StageCommand() *cli.Command {
	return &cli.Command{
		Name:            "stage",
		Aliases:         []string{"stg"},
		Usage:           "Manage staged changes for Google Cloud Secret Manager",
		Description:     stageDescription,
		Commands:        stageSubcommands(gcloudStageConfig()),
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// FlatStageCommand returns the Google Cloud stage command as a standalone
// top-level command named `name` (e.g. "stage"). Because there is no parent
// gcloud group to carry them, it folds in the --project flag and the
// project-resolving Before hook. Used for the flat `suve stage` alias when
// Google Cloud is the uniquely active staging provider.
func FlatStageCommand(name string) *cli.Command {
	return &cli.Command{
		Name:            name,
		Aliases:         []string{"stg"},
		Usage:           "Manage staged changes for Google Cloud Secret Manager",
		Description:     stageDescription,
		Flags:           projectFlags(),
		Before:          resolveProject,
		Commands:        stageSubcommands(gcloudStageConfig()),
		CommandNotFound: cliinternal.CommandNotFound,
	}
}
