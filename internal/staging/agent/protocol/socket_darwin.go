//go:build darwin

package protocol

import (
	"os"
	"path/filepath"
)

// SocketPath returns the path for the daemon socket on macOS.
func SocketPath() string {
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		return filepath.Join(tmpdir, "suve", "agent.sock")
	}
	return socketPathFallback()
}
