//go:build production || dev

package gui

import (
	_ "embed"
	"embed"
)

//go:embed all:frontend/dist
var Assets embed.FS

//go:embed appicon.png
var AppIcon []byte
