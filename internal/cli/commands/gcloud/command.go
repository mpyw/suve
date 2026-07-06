// Package gcloud provides CLI commands for Google Cloud Secret Manager,
// exposed as the "suve gcloud secret <op>" command group plus the
// "suve gcloud stage <op>" staging workflow.
//
// Google Cloud is secret-only (no parameter store). The read/write/tag commands
// (show, log, list, diff, create, update, delete, tag, untag) and the staging
// commands reuse the same generic scaffolding as their AWS counterparts via
// Google Cloud-specific presenters, use cases, and staging strategy.
package gcloud

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

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
			SecretCommand(),
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
	c := SecretCommand()
	c.Name = name
	c.Flags = projectFlags()
	c.Before = resolveProject

	return c
}

// projectFlags returns the shared --project flag (a fresh slice per call so
// each command owns its flag instance).
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
func resolveProject(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	project := cmd.String("project")
	if project == "" {
		project = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	return cliinternal.WithGoogleCloudProject(ctx, project), nil
}

// SecretCommand returns the "gcloud secret" subcommand group.
func SecretCommand() *cli.Command {
	return &cli.Command{
		Name:    "secret",
		Aliases: []string{"secrets", "sm"},
		Usage:   "Interact with Google Cloud Secret Manager secrets",
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
