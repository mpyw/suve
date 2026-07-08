//go:build production || dev

package main

import (
	"log"

	"github.com/mpyw/suve/internal/gui"
	"github.com/mpyw/suve/internal/provider"
)

func main() {
	// The standalone Wails dev/build entry resolves the initial provider from
	// the environment (same as a bare `suve --gui`); it carries no explicit
	// launch scope/service, so the GUI falls back to its env-derived default.
	if err := gui.Run(provider.Scope{Provider: gui.InitialProviderFromEnv()}, ""); err != nil {
		log.Fatal("Error: ", err.Error())
	}
}
