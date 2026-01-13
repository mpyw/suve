package ipc

import (
	"bytes"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

const (
	testAccountID = "123456789012"
	testRegion    = "us-east-1"
)

func TestNewServer(t *testing.T) {
	t.Parallel()

	handler := func(req *protocol.Request) *protocol.Response {
		return &protocol.Response{Success: true}
	}
	callback := func(req *protocol.Request, resp *protocol.Response) {}
	shutdownCb := func() {}

	s := NewServer(testAccountID, testRegion, handler, callback, shutdownCb)
	require.NotNil(t, s)
	assert.Equal(t, testAccountID, s.accountID)
	assert.Equal(t, testRegion, s.region)
	assert.NotNil(t, s.handler)
	assert.NotNil(t, s.onResponse)
	assert.NotNil(t, s.onShutdown)
	assert.NotNil(t, s.ctx)
	assert.NotNil(t, s.cancel)
}

func TestServer_Done(t *testing.T) {
	t.Parallel()

	handler := func(req *protocol.Request) *protocol.Response {
		return &protocol.Response{Success: true}
	}

	s := NewServer(testAccountID, testRegion, handler, nil, nil)

	// Channel should not be closed initially
	select {
	case <-s.Done():
		t.Fatal("done channel should not be closed initially")
	default:
		// Expected
	}

	// After shutdown, channel should be closed
	s.Shutdown()

	select {
	case <-s.Done():
		// Expected
	case <-time.After(time.Second):
		t.Fatal("done channel should be closed after shutdown")
	}
}

func TestServer_Shutdown(t *testing.T) {
	t.Parallel()

	t.Run("shutdown without listener", func(t *testing.T) {
		t.Parallel()

		handler := func(req *protocol.Request) *protocol.Response {
			return &protocol.Response{Success: true}
		}

		s := NewServer(testAccountID, testRegion, handler, nil, nil)
		// Shutdown should work even without a listener
		s.Shutdown()

		select {
		case <-s.Done():
			// Expected - context should be cancelled
		case <-time.After(time.Second):
			t.Fatal("context should be cancelled after shutdown")
		}
	})

	t.Run("shutdown with listener", func(t *testing.T) {
		t.Parallel()

		handler := func(req *protocol.Request) *protocol.Response {
			return &protocol.Response{Success: true}
		}

		s := NewServer(testAccountID, testRegion, handler, nil, nil)

		// Create a temporary listener using TCP for easier testing
		// (Unix socket paths have length limits on some platforms)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		s.listener = listener

		// Shutdown should close the listener
		s.Shutdown()

		// Verify listener is closed
		_, err = listener.Accept()
		assert.Error(t, err)
	})
}

func TestServer_sendError(t *testing.T) {
	t.Parallel()

	handler := func(req *protocol.Request) *protocol.Response {
		return &protocol.Response{Success: true}
	}
	s := NewServer(testAccountID, testRegion, handler, nil, nil)

	// Use a pipe to capture the response
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	errMsg := "test error message"
	go s.sendError(server, errMsg)

	var resp protocol.Response
	decoder := json.NewDecoder(client)
	err := decoder.Decode(&resp)
	require.NoError(t, err)

	assert.False(t, resp.Success)
	assert.Equal(t, errMsg, resp.Error)
}

// mockConn is a mock net.Conn that wraps a buffer for testing.
type mockConn struct {
	*bytes.Buffer
}

func (m *mockConn) Close() error                     { return nil }
func (m *mockConn) LocalAddr() net.Addr              { return nil }
func (m *mockConn) RemoteAddr() net.Addr             { return nil }
func (m *mockConn) SetDeadline(time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(time.Time) error { return nil }

func TestServer_sendError_withMockConn(t *testing.T) {
	t.Parallel()

	handler := func(req *protocol.Request) *protocol.Response {
		return &protocol.Response{Success: true}
	}
	s := NewServer(testAccountID, testRegion, handler, nil, nil)

	buf := &bytes.Buffer{}
	conn := &mockConn{Buffer: buf}

	s.sendError(conn, "error from mock")

	var resp protocol.Response
	err := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "error from mock", resp.Error)
}
