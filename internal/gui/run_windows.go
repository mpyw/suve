//go:build (production || dev) && windows

package gui

import "github.com/wailsapp/wails/v2/pkg/options"

func applyPlatformOptions(_ *options.App) {
	// Windows: No platform-specific options needed.
	// Icon is embedded via resource_windows_*.syso files generated during CI build.
}
