//go:build !linux

package agent

import (
	"net"
)

// setupProcessSecurityPlatform is a no-op on non-Linux platforms.
// macOS and Windows rely on socket/file permissions for security.
func setupProcessSecurityPlatform() error {
	return nil
}

// verifyPeerCredentialsPlatform is a no-op on non-Linux platforms.
// SO_PEERCRED is Linux-specific; macOS and Windows rely on socket permissions.
func verifyPeerCredentialsPlatform(_ net.Conn) error {
	return nil
}
