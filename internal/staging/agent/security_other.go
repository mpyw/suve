//go:build !linux && !darwin

package agent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// getSocketPath returns the path for the daemon socket on other platforms.
func getSocketPath() string {
	// Fallback to /tmp/suve-$UID/
	return filepath.Join(fmt.Sprintf("/tmp/suve-%d", os.Getuid()), "agent.sock")
}

// setupProcessSecurity is a no-op on unsupported platforms.
func (d *Daemon) setupProcessSecurity() error {
	return nil
}

// verifyPeerCredentials is a no-op on unsupported platforms.
func (d *Daemon) verifyPeerCredentials(_ net.Conn) error {
	return nil
}
