package agent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// createSocketDir creates the socket directory with secure permissions.
func createSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}
	// Ensure directory permissions are correct
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("failed to set socket directory permissions: %w", err)
	}
	return nil
}

// removeExistingSocket removes any existing socket file.
func removeExistingSocket(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}
	return nil
}

// setSocketPermissions sets secure permissions on the socket file.
func setSocketPermissions(socketPath string) error {
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}
	return nil
}

// getSocketPath returns the path for the daemon socket.
func getSocketPath() string {
	// Try XDG_RUNTIME_DIR first (Linux)
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, "suve", "agent.sock")
	}
	// Try TMPDIR (macOS)
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		return filepath.Join(tmpdir, "suve", "agent.sock")
	}
	// Fallback to /tmp/suve-$UID/
	return filepath.Join(fmt.Sprintf("/tmp/suve-%d", os.Getuid()), "agent.sock")
}

// verifyPeerCredentials checks that the connecting process has the same UID.
// This is a platform-specific function; see security_linux.go and security_other.go.
func verifyPeerCredentials(conn net.Conn) error {
	return verifyPeerCredentialsPlatform(conn)
}

// setupProcessSecurity configures process-level security measures.
// This is a platform-specific function; see security_linux.go and security_other.go.
func setupProcessSecurity() error {
	return setupProcessSecurityPlatform()
}
