package main

import (
	"context"
	"os"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/output"
)

func main() {
	// Register GUI flag (only effective when built with -tags production)
	registerGUIFlag()
	registerGUIDescription()
	if err := commands.App.Run(context.Background(), os.Args); err != nil {
		output.Error(os.Stderr, "%v", err)
		os.Exit(1)
	}
}
