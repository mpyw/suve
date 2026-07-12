package main

import (
	"context"
	"os"
	"slices"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/updatecheck"
)

// isShellCompletion reports whether args contain the hidden
// --generate-shell-completion flag that urfave/cli passes on every shell TAB.
func isShellCompletion(args []string) bool {
	return slices.Contains(args, "--generate-shell-completion")
}

func main() {
	// Register GUI flag (only effective when built with -tags production)
	registerGUIFlag()

	// Register the TUI --tui flag and usage. RegisterTUIFlag runs after
	// registerGUIFlag so the --tui Before-wrapper sits outermost and chains to
	// the GUI wrapper (and then enableDebug). RegisterTUIDescription runs before
	// registerGUIDescription so the usage composes to "CLI/GUI/TUI" in the GUI
	// build ("CLI/TUI" in the default build, where the GUI rewrite is a no-op).
	commands.RegisterTUIFlag()
	commands.RegisterTUIDescription()

	registerGUIDescription()

	ctx := context.Background()

	runErr := commands.App.Run(ctx, os.Args)

	// Notify-only, non-blocking update check. Runs on both the success and
	// error paths, always to STDERR so it never contaminates piped STDOUT.
	// It is bounded by a short HTTP timeout and cached for 24h, and returns
	// "" (silently) on any error or when disabled via SUVE_NO_UPDATE_CHECK.
	//
	// Skip it entirely for shell-completion invocations: those run on every
	// TAB and their STDERR is not redirected by the shell, so a notice would
	// garble the prompt and the HTTP timeout could block completion.
	if !isShellCompletion(os.Args) {
		if notice := updatecheck.Notice(ctx, commands.Version); notice != "" {
			output.Info(os.Stderr, "%s", notice)
		}
	}

	if runErr != nil {
		output.Error(os.Stderr, "%v", runErr)
		os.Exit(1)
	}
}
