//go:build (production || dev) && windows

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

func applyPlatformRunOptions(_ *application.Options) {
	// Windows: No platform-specific options needed.
	// Icon is embedded via resource_windows_*.syso files generated during CI build.
}
