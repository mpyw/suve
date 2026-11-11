//go:build (production || dev) && darwin

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

func applyPlatformRunOptions(_ *application.Options) {
	// macOS: No platform-specific options needed.
	// Dock icon requires .app bundle (wails build), which is not used here.
}
