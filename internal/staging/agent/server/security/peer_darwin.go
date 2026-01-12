//go:build darwin

package security

import "net"

// TODO: Implement macOS-specific peer credential verification using LOCAL_PEERCRED
// via getsockopt to verify the connecting process belongs to the same user.

// VerifyPeerCredentials is a no-op on macOS (not yet implemented).
func VerifyPeerCredentials(_ net.Conn) error { return nil }
