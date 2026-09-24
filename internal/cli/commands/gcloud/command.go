// Package gcloud provides CLI commands for Google Cloud Secret Manager,
// exposed as the "suve gcloud secret <op>" command group plus the
// "suve gcloud stage <op>" staging workflow.
//
// Google Cloud is secret-only (no parameter store). The read/write/tag commands
// live in the gcloud/secret package; the staging commands (stage.go) drive the
// shared staging scaffolding with the Google Cloud strategy. This package owns
// the --project flag and the Before hook that resolves it, so both subgroups
// see the same project.
//
// command.go is this package's subject: the gcloud command group it assembles,
// together with the flag and hook its sibling files share. Hence core.
//
//declscope:core
package gcloud

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	gcloudinternal "github.com/mpyw/suve/internal/cli/commands/gcloud/internal"
	"github.com/mpyw/suve/internal/cli/commands/gcloud/secret"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

// Command returns the gcloud command with the secret subcommand group.
func Command() *cli.Command {
	return &cli.Command{
		Name: "gcloud",
		// Intentional user-facing alias mirroring AWS's ssm/ps/sm shorthand;
		// the acronym ban is waived for this line only (see check-naming.sh).
		Aliases: []string{"gcp", "google"}, // naming-allow-gcp
		Usage:   "Interact with Google Cloud Secret Manager",
		Description: `Interact with Google Cloud Secret Manager.

Google Cloud secrets are integer-versioned (1, 2, 3, ... or "latest") and have
no staging labels. Set the project with --project or the GOOGLE_CLOUD_PROJECT
environment variable. Authentication uses Application Default Credentials.`,
		Flags: projectFlags(),
		// Before resolves the project once and stashes it in the context so the
		// generic command presenters (which do not receive *cli.Command) can
		// resolve a store. Resolution is deferred to store construction, so
		// `suve gcloud secret --help` still works without a project.
		Before: resolveProject,
		Commands: []*cli.Command{
			secret.Command(),
			StageCommand(),
		},
		CommandNotFound: cliinternal.CommandNotFound,
	}
}

// FlatSecretCommand returns the Google Cloud secret command as a standalone
// top-level command named `name` (e.g. "secret"). Because there is no parent
// gcloud group to carry them, it folds in the --project flag and the
// project-resolving Before hook. Used for the flat `suve secret` alias when
// Google Cloud is the uniquely active secret provider.
func FlatSecretCommand(name string) *cli.Command {
	c := secret.Command()
	c.Name = name
	c.Flags = projectFlags()
	c.Before = resolveProject

	return c
}

// projectFlags returns the shared --project flag (a fresh slice per call so
// each command owns its flag instance).
//
//declscope:package // stage.go builds the --project flag with it too
func projectFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "project",
			Usage: "Google Cloud project id (defaults to $GOOGLE_CLOUD_PROJECT)",
		},
	}
}

// resolveProject stashes the resolved project id (from --project or
// GOOGLE_CLOUD_PROJECT) into the context for the subcommands.
//
//declscope:package // stage.go uses it as the Before hook that resolves the project
func resolveProject(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	project := cmd.String("project")
	if project == "" {
		project = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	return gcloudinternal.WithProject(ctx, project), nil
}
