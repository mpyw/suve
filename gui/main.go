//go:build production || dev

package main

import (
	"log"

	"github.com/mpyw/suve/internal/gui"
)

func main() {
	// The standalone Wails dev/build entry resolves the initial provider from
	// the environment (same as a bare `suve --gui`).
	if err := gui.Run(gui.InitialProviderFromEnv()); err != nil {
		log.Fatal("Error: ", err.Error())
	}
}
