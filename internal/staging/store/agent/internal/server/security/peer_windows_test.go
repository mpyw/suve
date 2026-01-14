//go:build windows

package security_test

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/server/security"
)

func TestVerifyPeerCredentials_Windows_AlwaysSucceeds(t *testing.T) {
	t.Parallel()

	// On Windows, VerifyPeerCredentials is a no-op and always returns nil.
	// This is because Windows AF_UNIX sockets don't support peer credentials.

	// Test with nil connection
	err := security.VerifyPeerCredentials(nil)
	require.NoError(t, err)

	// Test with mock connection
	conn := &mockConn{}
	err = security.VerifyPeerCredentials(conn)
	require.NoError(t, err)
}

func TestVerifyPeerCredentials_Windows_DocumentedBehavior(t *testing.T) {
	t.Parallel()

	// Verify that the function accepts any connection type without error,
	// since security on Windows relies on socket file ACLs instead.
	connections := []net.Conn{nil, &mockConn{}}

	for _, conn := range connections {
		err := security.VerifyPeerCredentials(conn)
		assert.NoError(t, err, "VerifyPeerCredentials should always succeed on Windows")
	}
}

// mockConn is a minimal net.Conn implementation for testing.
type mockConn struct{}

func (m *mockConn) Read(_ []byte) (int, error)         { return 0, nil }
func (m *mockConn) Write(_ []byte) (int, error)        { return 0, nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(_ time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(_ time.Time) error { return nil }
