//go:build darwin

package agent

import (
	"net"
	"os"
	"path/filepath"
)

// getSocketPath returns the path for the daemon socket on macOS.
func getSocketPath() string {
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		return filepath.Join(tmpdir, "suve", "agent.sock")
	}
	return getSocketPathFallback()
}

// setupProcessSecurity is a no-op on macOS.
func (d *Daemon) setupProcessSecurity() error {
	return nil
}

// verifyPeerCredentials is a no-op on macOS.
func (d *Daemon) verifyPeerCredentials(_ net.Conn) error {
	return nil
}
