//go:build production || dev

package gui

import (
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Run starts the GUI application.
func Run() error {
	app := NewApp()

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
