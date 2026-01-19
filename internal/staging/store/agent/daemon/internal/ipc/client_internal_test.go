package ipc

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	c := NewClient()
	require.NotNil(t, c)
	assert.NotEmpty(t, c.socketPath)
	assert.Contains(t, c.socketPath, "agent.sock")
}

func TestClient_SendRequest_NotConnected(t *testing.T) {
	t.Parallel()

	// Use a non-existent socket path
	c := &Client{socketPath: "/nonexistent/path/to/socket.sock"}
	resp, err := c.SendRequest(t.Context(), &protocol.Request{Method: protocol.MethodPing})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrNotConnected)
}

func TestClient_Ping_NotConnected(t *testing.T) {
	t.Parallel()

	// Use a non-existent socket path
	c := &Client{socketPath: "/nonexistent/path/to/socket.sock"}
	err := c.Ping(t.Context())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotConnected)
}

//nolint:paralleltest // Uses t.Context() which cannot be used with t.Parallel().
func TestClient_SendRequest_Success(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-client-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a mock server
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		// Read request
		var req protocol.Request

		decoder := json.NewDecoder(conn)
		if err := decoder.Decode(&req); err != nil {
			return
		}

		// Send response
		resp := protocol.Response{Success: true}
		encoder := json.NewEncoder(conn)
		//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
		_ = encoder.Encode(&resp)
	}()

	// Create client with custom socket path
	c := &Client{socketPath: socketPath}

	resp, err := c.SendRequest(t.Context(), &protocol.Request{Method: protocol.MethodPing})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
}

//nolint:paralleltest // Uses t.Context() which cannot be used with t.Parallel().
func TestClient_SendRequest_ErrorResponse(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-client-err-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a mock server that returns an error
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		// Read request
		var req protocol.Request

		decoder := json.NewDecoder(conn)
		if err := decoder.Decode(&req); err != nil {
			return
		}

		// Send error response
		resp := protocol.Response{Success: false, Error: "test error"}
		encoder := json.NewEncoder(conn)
		//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
		_ = encoder.Encode(&resp)
	}()

	c := &Client{socketPath: socketPath}

	resp, err := c.SendRequest(t.Context(), &protocol.Request{Method: protocol.MethodPing})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "test error", resp.Error)
}

//nolint:paralleltest // Uses t.Context() which cannot be used with t.Parallel().
func TestClient_SendRequest_ServerClosesConnection(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-client-close-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a mock server that reads request but closes without response
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		// Read the request to allow client to complete write
		var req protocol.Request

		decoder := json.NewDecoder(conn)
		_ = decoder.Decode(&req)
		// Close without responding - client should get EOF on read
		_ = conn.Close()
	}()

	c := &Client{socketPath: socketPath}

	resp, err := c.SendRequest(t.Context(), &protocol.Request{Method: protocol.MethodPing})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "daemon closed connection unexpectedly")
}

//nolint:paralleltest // Uses t.Context() which cannot be used with t.Parallel().
func TestClient_Ping_Success(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-client-ping-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a mock server
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		// Read request
		var req protocol.Request

		decoder := json.NewDecoder(conn)
		if err := decoder.Decode(&req); err != nil {
			return
		}

		// Verify it's a ping request
		if req.Method != protocol.MethodPing {
			return
		}

		// Send success response
		resp := protocol.Response{Success: true}
		encoder := json.NewEncoder(conn)
		//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
		_ = encoder.Encode(&resp)
	}()

	c := &Client{socketPath: socketPath}

	err = c.Ping(t.Context())
	require.NoError(t, err)
}

//nolint:paralleltest // Uses t.Context() which cannot be used with t.Parallel().
func TestClient_Ping_ServerError(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-client-ping-err-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a mock server that returns error
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		// Read request
		var req protocol.Request

		decoder := json.NewDecoder(conn)
		if err := decoder.Decode(&req); err != nil {
			return
		}

		// Send error response
		resp := protocol.Response{Success: false, Error: "ping failed"}
		encoder := json.NewEncoder(conn)
		//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
		_ = encoder.Encode(&resp)
	}()

	c := &Client{socketPath: socketPath}

	err = c.Ping(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ping failed")
}

//nolint:paralleltest // Uses t.Context() which cannot be used with t.Parallel().
func TestClient_ConcurrentSendRequest(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-client-concurrent-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a mock server that handles multiple connections
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer func() { _ = c.Close() }()

				// Read request
				var req protocol.Request

				decoder := json.NewDecoder(c)
				if err := decoder.Decode(&req); err != nil {
					return
				}

				// Simulate some processing time
				time.Sleep(10 * time.Millisecond)

				// Send response
				resp := protocol.Response{Success: true}
				encoder := json.NewEncoder(c)
				//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
				_ = encoder.Encode(&resp)
			}(conn)
		}
	}()

	c := &Client{socketPath: socketPath}

	// Send multiple concurrent requests
	const numRequests = 10

	done := make(chan error, numRequests)

	for range numRequests {
		go func() {
			_, err := c.SendRequest(t.Context(), &protocol.Request{Method: protocol.MethodPing})
			done <- err
		}()
	}

	// Wait for all requests to complete
	for range numRequests {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent requests timed out")
		}
	}
}

//nolint:paralleltest // Uses t.Context() which cannot be used with t.Parallel().
func TestClient_SendRequest_DecodeNonEOFError(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-client-decode-err-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a mock server that returns invalid JSON
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		// Read request
		var req protocol.Request

		decoder := json.NewDecoder(conn)
		if err := decoder.Decode(&req); err != nil {
			return
		}

		// Send invalid JSON response (not EOF, just malformed)
		_, _ = conn.Write([]byte("{invalid json}\n"))
	}()

	c := &Client{socketPath: socketPath}

	resp, err := c.SendRequest(t.Context(), &protocol.Request{Method: protocol.MethodPing})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "failed to read response")
}

//nolint:paralleltest // Uses t.Context() which cannot be used with t.Parallel().
func TestClient_SendRequest_EncodeError(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-client-encode-err-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	socketPath := filepath.Join(tmpDir, "test.sock")

	// Start a mock server that immediately closes write direction
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	require.NoError(t, err)

	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		// Close immediately to cause encode failure
		_ = conn.Close()
	}()

	c := &Client{socketPath: socketPath}

	// This should fail when trying to encode the request because connection is closed
	resp, err := c.SendRequest(t.Context(), &protocol.Request{Method: protocol.MethodPing})
	require.Error(t, err)
	assert.Nil(t, resp)
	// The error should be either "failed to send request" or EOF-related
}
