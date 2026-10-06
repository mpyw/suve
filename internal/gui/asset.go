//go:build production || dev

package gui

import (
	"embed"
)

// assets contains the embedded frontend build artifacts.
//
//go:embed all:frontend/dist
//declscope:shared // run.go serves it
var assets embed.FS
