//go:build production || dev

// Package main provides the suve CLI entry point.
package main

import (
	"context"
	"os"
	"strings"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/gui"
	"github.com/mpyw/suve/internal/provider"
)

// guiFlagName is the name of the --gui launch flag, attached to the root, each
// provider group, and the service and stage subgroups (see RegisterLaunchMode).
const guiFlagName = "gui"

// launchGUI runs the GUI with the given initial scope + service and exits. It
// is the short-circuit used by the --gui flags' Before hooks. service is the
// launched service ("param"/"secret", or "" when launched at the group level or
// bare), so the GUI can open on the matching view. The GUI hydrates empty
// resource fields from the environment (flag wins over env).
func launchGUI(ctx context.Context, initial provider.Scope, service string) (context.Context, error) {
	if err := gui.Run(initial, service); err != nil {
		return ctx, err
	}

	os.Exit(0)

	return ctx, nil
}

// registerGUIFlag registers --gui through commands.RegisterLaunchMode, the
// registration `suve --tui` shares.
//
//declscope:package // called from main.go
func registerGUIFlag() {
	commands.RegisterLaunchMode(commands.LaunchMode{
		Flag:       guiFlagName,
		RootUsage:  "Launch GUI mode (picks the active provider, or opens the in-app picker if none is unambiguous)",
		GroupUsage: "Launch GUI mode for this provider",
		// Bare `suve --gui`: launch with the uniquely-active provider when the
		// environment resolves one; otherwise (0 or 2+ active)
		// DetectInitialProvider returns "", and the GUI opens at its in-app
		// provider picker rather than erroring. No specific service.
		Bare: func(ctx context.Context) (context.Context, error) {
			return launchGUI(ctx, provider.Scope{Provider: gui.DetectInitialProvider()}, "")
		},
		Launch: launchGUI,
	})
}

// registerGUIDescription marks the root usage as CLI/GUI.
//
//declscope:package // called from main.go
func registerGUIDescription() {
	commands.App.Usage = strings.Replace(commands.App.Usage, "CLI", "CLI/GUI", 1)
}
