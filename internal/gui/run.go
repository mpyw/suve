//go:build production || dev

package gui

import (
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/mpyw/suve/internal/provider"
)

// Run starts the GUI application with the given initial launch scope. A zero
// Provider means "no explicit choice" — the frontend falls back to env
// detection; a set Provider (with optional resource fields) pre-selects it.
func Run(initial provider.Scope) error {
	app := NewApp(initial)

	opts := &options.App{
		Title:  "suve",
		Width:  1024, //nolint:mnd
		Height: 768,  //nolint:mnd
		AssetServer: &assetserver.Options{
			Assets: Assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1}, //nolint:mnd
		OnStartup:        app.Startup,
		Bind: []any{
			app,
		},
	}
	applyPlatformOptions(opts)

	return wails.Run(opts)
}
