//go:build linux

package security

import (
	"fmt"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// VerifyPeerCredentials checks peer credentials on Linux using SO_PEERCRED.
func VerifyPeerCredentials(conn net.Conn) error {
	// Use SyscallConn to access the raw file descriptor without disrupting the connection.
	// Unlike File(), this doesn't duplicate the fd or change the connection to blocking mode.
	syscallConn, ok := conn.(syscall.Conn)
	if !ok {
		return fmt.Errorf("connection does not support syscall.Conn")
	}

	rawConn, err := syscallConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("failed to get raw connection: %w", err)
	}

	var peerUID uint32

	var credErr error

	controlErr := rawConn.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			credErr = fmt.Errorf("failed to get peer credentials: %w", err)

			return
		}

		peerUID = cred.Uid
	})
	if controlErr != nil {
		return fmt.Errorf("failed to access socket: %w", controlErr)
	}

	if credErr != nil {
		return credErr
	}

	//nolint:gosec // G115: UID is always non-negative and fits in uint32
	if peerUID != uint32(os.Getuid()) {
		return fmt.Errorf("permission denied: peer UID %d does not match %d", peerUID, os.Getuid())
	}

	return nil
}
