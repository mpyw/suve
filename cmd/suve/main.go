package main

import (
	"context"
	"os"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/updatecheck"
)

func main() {
	// Register GUI flag (only effective when built with -tags production)
	registerGUIFlag()
	registerGUIDescription()

	ctx := context.Background()

	runErr := commands.App.Run(ctx, os.Args)

	// Notify-only, non-blocking update check. Runs on both the success and
	// error paths, always to STDERR so it never contaminates piped STDOUT.
	// It is bounded by a short HTTP timeout and cached for 24h, and returns
	// "" (silently) on any error or when disabled via SUVE_NO_UPDATE_CHECK.
	if notice := updatecheck.Notice(ctx, commands.Version); notice != "" {
		output.Info(os.Stderr, "%s", notice)
	}

	if runErr != nil {
		output.Error(os.Stderr, "%v", runErr)
		os.Exit(1)
	}
}
