//go:build production || dev

// Package main provides the suve CLI entry point.
package main

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/gui"
	"github.com/mpyw/suve/internal/provider"
)

// errInvalidGUIProvider is returned when a bare `suve --gui` cannot resolve a
// single active provider from the environment (0 or 2+ active). The user must
// choose explicitly with `suve <provider> --gui`.
var errInvalidGUIProvider = errors.New(
	"--gui: cannot determine a unique active provider from the environment; " +
		"launch a specific one with `suve aws --gui`, `suve gcloud --gui`, or `suve azure --gui`",
)

// launchGUI runs the GUI with the given initial provider and exits. It is the
// short-circuit used by the --gui flags' Before hooks.
func launchGUI(ctx context.Context, initial provider.Provider) (context.Context, error) {
	if err := gui.Run(initial); err != nil {
		return ctx, err
	}

	os.Exit(0)

	return ctx, nil
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
		Usage: "Launch GUI mode (bare form picks the uniquely-active provider)",
	})
	commands.App.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Bool("gui") {
			initial := gui.InitialProviderFromEnv()
			if initial == "" {
				return ctx, errInvalidGUIProvider
			}

			return launchGUI(ctx, initial)
		}

		return ctx, nil
	}

	// Per-provider `suve <group> --gui`: launch with that provider pre-selected.
	for _, group := range commands.App.Commands {
		p := groupProvider(group.Name)
		if p == "" {
			continue
		}

		attachGUIFlag(group, p)
	}
}

// attachGUIFlag adds a --gui flag to a provider group and wraps its Before hook
// so `suve <group> --gui` launches the GUI with that provider.
func attachGUIFlag(group *cli.Command, p provider.Provider) {
	group.Flags = append(group.Flags, &cli.BoolFlag{
		Name:  "gui",
		Usage: "Launch GUI mode for this provider",
	})

	inner := group.Before
	group.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Bool("gui") {
			return launchGUI(ctx, p)
		}

		if inner != nil {
			return inner(ctx, cmd)
		}

		return ctx, nil
	}
}

func registerGUIDescription() {
	commands.App.Usage = strings.Replace(commands.App.Usage, "CLI", "CLI/GUI", 1)
}
