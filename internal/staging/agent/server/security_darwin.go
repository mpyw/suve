//go:build darwin

package server

import "net"

// setupProcessSecurity is a no-op on macOS.
func (d *Daemon) setupProcessSecurity() error {
	return nil
}

// verifyPeerCredentials is a no-op on macOS.
func (d *Daemon) verifyPeerCredentials(_ net.Conn) error {
	return nil
}
