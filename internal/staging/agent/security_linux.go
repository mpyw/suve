//go:build linux

package agent

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// setupProcessSecurityPlatform configures Linux-specific security measures.
func setupProcessSecurityPlatform() error {
	// Disable core dumps to prevent memory leaks
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("failed to disable core dumps: %w", err)
	}
	return nil
}

// verifyPeerCredentialsPlatform checks peer credentials on Linux.
func verifyPeerCredentialsPlatform(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("connection is not a Unix socket")
	}

	file, err := unixConn.File()
	if err != nil {
		return fmt.Errorf("failed to get socket file descriptor: %w", err)
	}
	defer func() { _ = file.Close() }()

	cred, err := unix.GetsockoptUcred(int(file.Fd()), unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return fmt.Errorf("failed to get peer credentials: %w", err)
	}

	if cred.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("permission denied: peer UID %d does not match %d", cred.Uid, os.Getuid())
	}

	return nil
}
