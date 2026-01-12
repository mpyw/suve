package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// getSocketPath returns the path for the daemon socket.
func getSocketPath() string {
	// Try XDG_RUNTIME_DIR first (Linux)
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, "suve", "agent.sock")
	}
	// Try TMPDIR (macOS)
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		return filepath.Join(tmpdir, "suve", "agent.sock")
	}
	// Fallback to /tmp/suve-$UID/
	return filepath.Join(fmt.Sprintf("/tmp/suve-%d", os.Getuid()), "agent.sock")
}
