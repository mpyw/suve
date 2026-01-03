package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/gui"
)

func main() {
	// Check for --gui flag
	for _, arg := range os.Args[1:] {
		if arg == "--gui" {
			if err := gui.Run(); err != nil {
				log.Fatal("Error: ", err.Error())
			}
			return
		}
	}

	// Run CLI
	// Add --gui flag to commands.App.Flags so it appears in help
	commands.App.Flags = append(commands.App.Flags, &cli.BoolFlag{
		Name:  "gui",
		Usage: "Launch GUI mode",
	})
	if err := commands.App.Run(context.Background(), os.Args); err != nil {
		output.Error(os.Stderr, "%v", err)
		os.Exit(1)
	}
}
