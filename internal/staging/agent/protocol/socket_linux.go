//go:build linux

package protocol

import (
	"os"
	"path/filepath"
)

// SocketPath returns the path for the daemon socket on Linux.
func SocketPath() string {
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, "suve", "agent.sock")
	}
	return socketPathFallback()
}
