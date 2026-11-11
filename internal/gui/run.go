//go:build production || dev

package gui

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/mpyw/suve/internal/provider"
)

// Run starts the GUI application with the given initial launch scope and
// service. A zero Provider means "no explicit choice" — the frontend falls back
// to env detection; a set Provider (with optional resource fields) pre-selects
// it. service ("param"/"secret", or "" for none) is the launched service the
// frontend opens on.
func Run(initial provider.Scope, service string) error {
	svc, err := newApp(initial, service)
	if err != nil {
		return err
	}

	opts := application.Options{
		Name: "suve",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// Quit when the window closes, as Wails v2 did. v3 keeps a macOS app
		// running without windows unless this is set.
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	}
	applyPlatformRunOptions(&opts)

	app := application.New(opts)
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "suve",
		Width:            1024,                           //nolint:mnd
		Height:           768,                            //nolint:mnd
		BackgroundColour: application.NewRGB(27, 38, 54), //nolint:mnd
		URL:              "/",
	})

	return app.Run()
}
