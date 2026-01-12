//go:build linux

package agent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// getSocketPath returns the path for the daemon socket on Linux.
func getSocketPath() string {
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, "suve", "agent.sock")
	}
	return getSocketPathFallback()
}

// setupProcessSecurity configures Linux-specific security measures.
func (d *Daemon) setupProcessSecurity() error {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("failed to disable core dumps: %w", err)
	}
	return nil
}

// verifyPeerCredentials checks peer credentials on Linux.
func (d *Daemon) verifyPeerCredentials(conn net.Conn) error {
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
