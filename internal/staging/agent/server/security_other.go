//go:build !linux && !darwin

package server

import "net"

func (d *Daemon) setupProcessSecurity() error            { return nil }
func (d *Daemon) verifyPeerCredentials(_ net.Conn) error { return nil }
