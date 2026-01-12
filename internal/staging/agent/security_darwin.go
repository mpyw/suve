//go:build darwin

package agent

import (
	"net"
	"os"
	"path/filepath"
)

// getSocketPath returns the path for the daemon socket on macOS.
func getSocketPath() string {
	// Use TMPDIR (macOS standard, per-user secure directory)
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		return filepath.Join(tmpdir, "suve", "agent.sock")
	}
	// Fallback (should not happen on macOS)
	return filepath.Join("/tmp", "suve", "agent.sock")
}

// setupProcessSecurity is a no-op on macOS.
// macOS relies on socket/file permissions for security.
func (d *Daemon) setupProcessSecurity() error {
	return nil
}

// verifyPeerCredentials is a no-op on macOS.
// SO_PEERCRED is Linux-specific; macOS relies on socket permissions.
func (d *Daemon) verifyPeerCredentials(_ net.Conn) error {
	return nil
}
