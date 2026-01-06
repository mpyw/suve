//go:build (production || dev) && windows

package gui

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func applyPlatformOptions(opts *options.App) {
	opts.Windows = &windows.Options{
		WebviewIsTransparent: false,
		WindowIsTranslucent:  false,
	}
}
