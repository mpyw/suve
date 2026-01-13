//go:build !linux && !darwin && !windows

package security

import "net"

// VerifyPeerCredentials is a no-op on unsupported platforms.
func VerifyPeerCredentials(_ net.Conn) error {
	return nil
}
