//go:build production || dev

package gui

import (
	"embed"
)

// Assets contains the embedded frontend build artifacts.
//
//go:embed all:frontend/dist
var Assets embed.FS

// IconAsset contains the embedded application icon.
//
//go:embed appicon.png
var IconAsset []byte
