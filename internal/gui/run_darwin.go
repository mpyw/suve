//go:build (production || dev) && darwin

package gui

import "github.com/wailsapp/wails/v2/pkg/options"

func applyPlatformOptions(_ *options.App) {
	// macOS: No platform-specific options needed.
	// Dock icon requires .app bundle (wails build), which is not used here.
}
