//go:build linux

package protocol

import (
	"os"
	"path/filepath"
)

// socketPath returns the path for the daemon socket on Linux.
func socketPath() string {
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, socketDirName, socketFileName)
	}

	return socketPathFallback()
}
