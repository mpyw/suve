//go:build !linux && !darwin

package agent

import "net"

func getSocketPath() string                              { return getSocketPathFallback() }
func (d *Daemon) setupProcessSecurity() error            { return nil }
func (d *Daemon) verifyPeerCredentials(_ net.Conn) error { return nil }
