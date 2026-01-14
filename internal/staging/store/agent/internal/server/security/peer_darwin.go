//go:build darwin

package security

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	localPeerCred    = 0x001 // LOCAL_PEERCRED
	xucredSize       = 76    // sizeof(struct xucred) on Darwin
	xucredUIDEndByte = 8     // End of cr_uid field in xucred structure
)

// VerifyPeerCredentials checks peer credentials on macOS using LOCAL_PEERCRED.
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
		// struct xucred layout:
		// - uint32 cr_version (4 bytes)
		// - uid_t  cr_uid     (4 bytes)
		// - short  cr_ngroups (2 bytes)
		// - gid_t  cr_groups[16] (64 bytes)
		// Total: 74 bytes, but sizeof reports 76 due to alignment
		buf := make([]byte, xucredSize)
		bufLen := uint32(xucredSize)

		// Use raw getsockopt syscall because GetsockoptString doesn't work for binary structures.
		//nolint:staticcheck,gosec // SA1019: Syscall6 needed for getsockopt; G103: unsafe required for syscall
		_, _, errno := unix.Syscall6(
			unix.SYS_GETSOCKOPT,
			fd,
			unix.SOL_LOCAL,
			localPeerCred,
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&bufLen)),
			0,
		)

		if errno != 0 {
			credErr = fmt.Errorf("failed to get peer credentials: %w", errno)

			return
		}

		if bufLen < xucredUIDEndByte {
			credErr = fmt.Errorf("invalid peer credentials: buffer too small")

			return
		}

		// Extract UID from bytes 4-8 (little-endian on all Apple platforms)
		peerUID = binary.LittleEndian.Uint32(buf[4:xucredUIDEndByte])
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
