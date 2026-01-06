//go:build production || dev

package main

import (
	"context"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/gui"
)

// runGUIIfRequested is kept for interface compatibility with gui_stub.go.
// GUI is now launched via Before hook in registerGUIFlag().
func runGUIIfRequested() bool {
	return false
}

func registerGUIFlag() {
	commands.App.Flags = append(commands.App.Flags, &cli.BoolFlag{
		Name:  "gui",
		Usage: "Launch GUI mode",
	})
	commands.App.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		if cmd.Bool("gui") {
			if err := gui.Run(); err != nil {
				return ctx, err
			}
			os.Exit(0)
		}
		return ctx, nil
	}
}

func registerGUIDescription() {
	commands.App.Usage = strings.Replace(commands.App.Usage, "CLI", "CLI/GUI", 1)
}
