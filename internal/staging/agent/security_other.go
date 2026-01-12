//go:build !linux

package agent

import (
	"net"
)

// setupProcessSecurity is a no-op on non-Linux platforms.
// macOS and Windows rely on socket/file permissions for security.
func (d *Daemon) setupProcessSecurity() error {
	return nil
}

// verifyPeerCredentials is a no-op on non-Linux platforms.
// SO_PEERCRED is Linux-specific; macOS and Windows rely on socket permissions.
func (d *Daemon) verifyPeerCredentials(_ net.Conn) error {
	return nil
}
