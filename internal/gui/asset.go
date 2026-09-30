//go:build production || dev

package gui

import (
	"embed"
)

// assets contains the embedded frontend build artifacts.
//
//go:embed all:frontend/dist
//declscope:package // run.go serves it
var assets embed.FS

// iconAsset contains the embedded application icon.
//
//go:embed appicon.png
//declscope:package // run_linux.go sets the window icon with it
var iconAsset []byte //nolint:unused // only run_linux.go uses it, so other hosts see no use
