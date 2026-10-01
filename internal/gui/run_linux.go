//go:build (production || dev) && linux

package gui

import (
	_ "embed"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// runWindowIcon is the embedded application icon. Only the Linux backend sets a
// window icon, so only the Linux build embeds it.
//
//go:embed appicon.png
var runWindowIcon []byte

func applyPlatformRunOptions(opts *options.App) {
	opts.Linux = &linux.Options{
		Icon: runWindowIcon,
	}
}
