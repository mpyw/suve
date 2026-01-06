//go:build (production || dev) && linux

package gui

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

func applyPlatformOptions(opts *options.App) {
	opts.Linux = &linux.Options{
		Icon: AppIcon,
	}
}
