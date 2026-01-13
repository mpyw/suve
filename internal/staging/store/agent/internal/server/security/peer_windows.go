//go:build windows

package security

import "net"

// TODO: Implement Windows-specific peer verification.
// Windows AF_UNIX sockets don't support traditional peer credentials.
// Consider using named pipes with security descriptors instead.

// VerifyPeerCredentials is a no-op on Windows (not yet implemented).
func VerifyPeerCredentials(_ net.Conn) error { return nil }
