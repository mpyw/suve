//go:build production || dev

package main

import (
	"log"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/gui"
)

func runGUIIfRequested() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--gui" {
			if err := gui.Run(); err != nil {
				log.Fatal("Error: ", err.Error())
			}
			return true
		}
	}
	return false
}

func registerGUIFlag() {
	commands.App.Flags = append(commands.App.Flags, &cli.BoolFlag{
		Name:  "gui",
		Usage: "Launch GUI mode",
	})
}

func registerGUIDescription() {
	commands.App.Usage = strings.Replace(commands.App.Usage, "CLI", "CLI/GUI", 1)
}
