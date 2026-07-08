//go:build production || dev

// Package main provides the suve CLI entry point.
package main

import (
	"context"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/gui"
	"github.com/mpyw/suve/internal/provider"
)

// launchGUI runs the GUI with the given initial scope and exits. It is the
// short-circuit used by the --gui flags' Before hooks.
func launchGUI(ctx context.Context, initial provider.Scope) (context.Context, error) {
	if err := gui.Run(initial); err != nil {
		return ctx, err
	}

	os.Exit(0)

	return ctx, nil
}

// guiScope builds the initial GUI launch scope for provider p from the flags
// present on the launching command: --project (Google Cloud), --vault-name /
// --store-name (Azure). Absent flags stay empty and are hydrated from the
// environment inside the GUI. AWS carries no scope flag (region from config).
func guiScope(cmd *cli.Command, p provider.Provider) provider.Scope {
	s := provider.Scope{Provider: p}

	switch p {
	case provider.ProviderGoogleCloud:
		s.ProjectID = cmd.String("project")
	case provider.ProviderAzure:
		s.VaultName = cmd.String("vault-name")
		s.StoreName = cmd.String("store-name")
	case provider.ProviderAWS:
		// region comes from the ambient AWS config; no scope flag.
	}

	return s
}

// groupProvider maps a top-level command group name to its provider, or "" when
// the command is not a provider group.
func groupProvider(name string) provider.Provider {
	switch name {
	case "aws":
		return provider.ProviderAWS
	case "gcloud":
		return provider.ProviderGoogleCloud
	case "azure":
		return provider.ProviderAzure
	default:
		return ""
	}
}

func registerGUIFlag() {
	// Bare `suve --gui`: initial provider resolved from the environment.
	commands.App.Flags = append(commands.App.Flags, &cli.BoolFlag{
		Name:  "gui",
		Usage: "Launch GUI mode (picks the active provider, or opens the in-app picker if none is unambiguous)",
	})
	// Wrap (do not replace) the root Before: the app already installs one
	// (enableDebug), and clobbering it silently disables --debug / SUVE_DEBUG in
	// the GUI build. Chain to it on the non-GUI path, mirroring attachGUIFlag.
	inner := commands.App.Before
	commands.App.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Bool("gui") {
			// Launch with the uniquely-active provider when the environment
			// resolves one; otherwise (0 or 2+ active) InitialProviderFromEnv
			// returns "", and the GUI opens at its in-app provider picker rather
			// than erroring. No scope flags on the bare form, so the GUI hydrates
			// any resource fields from env.
			return launchGUI(ctx, provider.Scope{Provider: gui.InitialProviderFromEnv()})
		}

		if inner != nil {
			return inner(ctx, cmd)
		}

		return ctx, nil
	}

	// Azure's --vault-name / --store-name live on its secret / param subgroups.
	const (
		subcommandSecret = "secret"
		subcommandParam  = "param"
	)

	// Per-provider `suve <group> --gui`: launch with that provider pre-selected.
	// Azure attaches --gui to its secret / param subgroups too, letting those
	// flags seed the launch scope.
	for _, group := range commands.App.Commands {
		p := groupProvider(group.Name)
		if p == "" {
			continue
		}

		attachGUIFlag(group, p)

		if p == provider.ProviderAzure {
			for _, sub := range group.Commands {
				if sub.Name == subcommandSecret || sub.Name == subcommandParam {
					attachGUIFlag(sub, p)
				}
			}
		}
	}
}

// attachGUIFlag adds a --gui flag to a command (a provider group or one of its
// subgroups) and wraps its Before hook so `--gui` launches the GUI with that
// provider, seeding the initial scope from the command's scope flags.
func attachGUIFlag(cmd *cli.Command, p provider.Provider) {
	cmd.Flags = append(cmd.Flags, &cli.BoolFlag{
		Name:  "gui",
		Usage: "Launch GUI mode for this provider",
	})

	inner := cmd.Before
	cmd.Before = func(ctx context.Context, c *cli.Command) (context.Context, error) {
		if c.Bool("gui") {
			return launchGUI(ctx, guiScope(c, p))
		}

		if inner != nil {
			return inner(ctx, c)
		}

		return ctx, nil
	}
}

func registerGUIDescription() {
	commands.App.Usage = strings.Replace(commands.App.Usage, "CLI", "CLI/GUI", 1)
}
