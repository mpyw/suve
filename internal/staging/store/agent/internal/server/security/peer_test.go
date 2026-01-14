//go:build darwin || linux

package security_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/server/security"
)

func TestVerifyPeerCredentials_ValidConnection(t *testing.T) {
	t.Parallel()

	// Create temporary socket
	tmpDir, err := os.MkdirTemp("", "peer-test-*")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start listener with test context (auto-cancelled on test end)
	var lc net.ListenConfig

	listener, err := lc.Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	// Accept connection in goroutine
	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errCh <- err

			return
		}

		connCh <- conn
	}()

	// Connect as client
	dialer := &net.Dialer{Timeout: time.Second}

	clientConn, err := dialer.DialContext(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = clientConn.Close() }()

	// Wait for server connection
	select {
	case serverConn := <-connCh:
		defer func() { _ = serverConn.Close() }()

		// Verify peer credentials - should succeed since same user
		err = security.VerifyPeerCredentials(serverConn)
		require.NoError(t, err)
	case err := <-errCh:
		t.Fatalf("Accept failed: %v", err)
	case <-t.Context().Done():
		t.Fatal("Test context cancelled")
	}
}

func TestVerifyPeerCredentials_NonSyscallConn(t *testing.T) {
	t.Parallel()

	// Create a mock connection that doesn't implement syscall.Conn
	conn := &mockNonSyscallConn{}

	err := security.VerifyPeerCredentials(conn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support syscall.Conn")
}

// mockNonSyscallConn is a minimal net.Conn that doesn't implement syscall.Conn.
type mockNonSyscallConn struct{}

func (m *mockNonSyscallConn) Read(_ []byte) (int, error)         { return 0, nil }
func (m *mockNonSyscallConn) Write(_ []byte) (int, error)        { return 0, nil }
func (m *mockNonSyscallConn) Close() error                       { return nil }
func (m *mockNonSyscallConn) LocalAddr() net.Addr                { return nil }
func (m *mockNonSyscallConn) RemoteAddr() net.Addr               { return nil }
func (m *mockNonSyscallConn) SetDeadline(_ time.Time) error      { return nil }
func (m *mockNonSyscallConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *mockNonSyscallConn) SetWriteDeadline(_ time.Time) error { return nil }
