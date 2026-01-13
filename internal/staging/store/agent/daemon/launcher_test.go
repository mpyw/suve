//nolint:testpackage // Internal tests for unexported functions (startProcess, mockSpawner)
package daemon

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging/store/agent/daemon/internal/ipc"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

const (
	testAccountID = "123456789012"
	testRegion    = "us-east-1"
)

// mockSpawner is a test mock for processSpawner.
type mockSpawner struct {
	spawnFunc  func(accountID, region string) error
	spawnCount atomic.Int32
}

func (m *mockSpawner) Spawn(accountID, region string) error {
	m.spawnCount.Add(1)

	if m.spawnFunc != nil {
		return m.spawnFunc(accountID, region)
	}

	return nil
}

func TestNewLauncher(t *testing.T) {
	t.Parallel()

	t.Run("default options", func(t *testing.T) {
		t.Parallel()

		l := NewLauncher(testAccountID, testRegion)
		require.NotNil(t, l)
		assert.NotNil(t, l.client)
		assert.Equal(t, testAccountID, l.accountID)
		assert.Equal(t, testRegion, l.region)
		assert.False(t, l.autoStartDisabled)
	})

	t.Run("with auto start disabled", func(t *testing.T) {
		t.Parallel()

		l := NewLauncher(testAccountID, testRegion, WithAutoStartDisabled())
		require.NotNil(t, l)
		assert.True(t, l.autoStartDisabled)
	})
}

func TestLauncher_startProcess(t *testing.T) {
	t.Parallel()

	t.Run("auto start disabled returns error", func(t *testing.T) {
		t.Parallel()

		l := NewLauncher(testAccountID, testRegion, WithAutoStartDisabled())

		err := l.startProcess()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "daemon not running and auto-start is disabled")
		assert.Contains(t, err.Error(), "suve stage agent start")
		assert.Contains(t, err.Error(), "--account")
		assert.Contains(t, err.Error(), "--region")
	})
}

func TestLauncher_EnsureRunning(t *testing.T) {
	t.Parallel()

	t.Run("daemon not running and auto start disabled", func(t *testing.T) {
		t.Parallel()

		l := NewLauncher(testAccountID, testRegion, WithAutoStartDisabled())

		// EnsureRunning should fail because daemon is not running
		// and auto-start is disabled
		err := l.EnsureRunning()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start daemon")
	})
}

func TestLauncher_Ping(t *testing.T) {
	t.Parallel()

	t.Run("daemon not running", func(t *testing.T) {
		t.Parallel()

		l := NewLauncher(testAccountID, testRegion, WithAutoStartDisabled())

		err := l.Ping()
		require.Error(t, err)
		// Should contain "not connected" from the IPC client
		assert.Contains(t, err.Error(), "not connected")
	})
}

func TestLauncher_Shutdown(t *testing.T) {
	t.Parallel()

	t.Run("daemon not running", func(t *testing.T) {
		t.Parallel()

		l := NewLauncher(testAccountID, testRegion, WithAutoStartDisabled())

		err := l.Shutdown()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not connected")
	})
}

func TestLauncher_SendRequest(t *testing.T) {
	t.Parallel()

	t.Run("daemon not running", func(t *testing.T) {
		t.Parallel()

		l := NewLauncher(testAccountID, testRegion, WithAutoStartDisabled())

		resp, err := l.SendRequest(&protocol.Request{Method: protocol.MethodPing})
		require.Error(t, err)
		assert.Nil(t, resp)
	})
}

func TestLauncher_EnsureRunning_AlreadyRunning(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-launcher-already-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "b1"
	region := "r1"

	socketPath := protocol.SocketPathForAccount(accountID, region)

	// Create socket directory
	err = os.MkdirAll(filepath.Dir(socketPath), 0o700)
	require.NoError(t, err)

	// Start a mock server
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

				var req protocol.Request

				decoder := json.NewDecoder(c)

				if err := decoder.Decode(&req); err != nil {
					return
				}

				resp := protocol.Response{Success: true}
				encoder := json.NewEncoder(c)
				//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
				_ = encoder.Encode(&resp)
			}(conn)
		}
	}()

	// EnsureRunning should succeed immediately (daemon already running)
	l := NewLauncher(accountID, region, WithAutoStartDisabled())
	err = l.EnsureRunning()
	require.NoError(t, err)
}

func TestLauncher_MultipleOptions(t *testing.T) {
	t.Parallel()

	// Test that options are applied in order
	l := NewLauncher(testAccountID, testRegion,
		WithAutoStartDisabled(),
	)
	require.NotNil(t, l)
	assert.True(t, l.autoStartDisabled)
}

func TestLauncher_PingWithRunningDaemon(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-launcher-ping-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "b2"
	region := "r2"

	socketPath := protocol.SocketPathForAccount(accountID, region)

	// Create socket directory
	err = os.MkdirAll(filepath.Dir(socketPath), 0o700)
	require.NoError(t, err)

	// Start a mock server
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

				var req protocol.Request

				decoder := json.NewDecoder(c)

				if err := decoder.Decode(&req); err != nil {
					return
				}

				resp := protocol.Response{Success: true}
				encoder := json.NewEncoder(c)
				//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
				_ = encoder.Encode(&resp)
			}(conn)
		}
	}()

	l := NewLauncher(accountID, region, WithAutoStartDisabled())
	err = l.Ping()
	require.NoError(t, err)
}

func TestLauncher_ShutdownWithRunningDaemon(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-launcher-shutdown-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "b3"
	region := "r3"

	socketPath := protocol.SocketPathForAccount(accountID, region)

	// Create socket directory
	err = os.MkdirAll(filepath.Dir(socketPath), 0o700)
	require.NoError(t, err)

	// Start a mock server
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

				var req protocol.Request

				decoder := json.NewDecoder(c)

				if err := decoder.Decode(&req); err != nil {
					return
				}

				resp := protocol.Response{Success: true}
				encoder := json.NewEncoder(c)
				//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
				_ = encoder.Encode(&resp)
			}(conn)
		}
	}()

	l := NewLauncher(accountID, region, WithAutoStartDisabled())
	err = l.Shutdown()
	require.NoError(t, err)
}

func TestLauncher_ShutdownWithServerError(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-launcher-shutdown-err-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "b4"
	region := "r4"

	socketPath := protocol.SocketPathForAccount(accountID, region)

	// Create socket directory
	err = os.MkdirAll(filepath.Dir(socketPath), 0o700)
	require.NoError(t, err)

	// Start a mock server that returns an error
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

				var req protocol.Request

				decoder := json.NewDecoder(c)

				if err := decoder.Decode(&req); err != nil {
					return
				}

				resp := protocol.Response{Success: false, Error: "shutdown failed"}
				encoder := json.NewEncoder(c)
				//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
				_ = encoder.Encode(&resp)
			}(conn)
		}
	}()

	l := NewLauncher(accountID, region, WithAutoStartDisabled())
	err = l.Shutdown()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown failed")
}

func TestLauncher_EnsureRunning_Timeout(t *testing.T) {
	// Create temp directory for socket - use short timeout test
	tmpDir, err := os.MkdirTemp("/tmp", "suve-launcher-timeout-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "b5"
	region := "r5"

	// Don't create a socket - let it timeout
	l := NewLauncher(accountID, region, WithAutoStartDisabled())

	// This should fail because daemon is not running and auto-start is disabled
	start := time.Now()
	err = l.EnsureRunning()
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start daemon")

	// The error should be quick since auto-start is disabled
	assert.Less(t, elapsed, 1*time.Second)
}

// TestLauncher_ClientIntegration verifies that the IPC client is properly constructed.
func TestLauncher_ClientIntegration(t *testing.T) {
	t.Parallel()

	l := NewLauncher(testAccountID, testRegion)
	require.NotNil(t, l.client)

	// Verify the client is an IPC client
	_ = ipc.NewClient(testAccountID, testRegion) // Just verify the import works
}

// TestLauncher_EnsureRunning_WithMockSpawner tests EnsureRunning with a mock spawner.
func TestLauncher_EnsureRunning_WithMockSpawner(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-launcher-mock-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "m1"
	region := "r1"

	socketPath := protocol.SocketPathForAccount(accountID, region)

	// Create socket directory
	err = os.MkdirAll(filepath.Dir(socketPath), 0o700)
	require.NoError(t, err)

	// Create mock spawner that starts a mock server when called
	spawner := &mockSpawner{}

	var listener net.Listener

	spawner.spawnFunc = func(aid, reg string) error {
		// Verify correct parameters
		assert.Equal(t, accountID, aid)
		assert.Equal(t, region, reg)

		// Start mock server
		var err error

		listener, err = (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
		if err != nil {
			return err
		}

		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}

				go func(c net.Conn) {
					defer func() { _ = c.Close() }()

					var req protocol.Request

					decoder := json.NewDecoder(c)

					if err := decoder.Decode(&req); err != nil {
						return
					}

					resp := protocol.Response{Success: true}
					encoder := json.NewEncoder(c)
					//nolint:errchkjson // Test code: Response struct is safe for JSON encoding
					_ = encoder.Encode(&resp)
				}(conn)
			}
		}()

		return nil
	}

	defer func() {
		if listener != nil {
			_ = listener.Close()
		}
	}()

	l := NewLauncher(accountID, region, withSpawner(spawner))

	// EnsureRunning should start the daemon via spawner and then ping successfully
	err = l.EnsureRunning()
	require.NoError(t, err)

	// Verify spawner was called
	assert.Equal(t, int32(1), spawner.spawnCount.Load())
}

// TestLauncher_EnsureRunning_SpawnError tests EnsureRunning when spawner fails.
func TestLauncher_EnsureRunning_SpawnError(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-launcher-spawn-err-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "m2"
	region := "r2"

	spawner := &mockSpawner{
		spawnFunc: func(_, _ string) error {
			return errors.New("spawn failed")
		},
	}

	l := NewLauncher(accountID, region, withSpawner(spawner))

	err = l.EnsureRunning()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start daemon")
	assert.Contains(t, err.Error(), "spawn failed")
}

// TestLauncher_EnsureRunning_TimeoutWithMockSpawner tests EnsureRunning timeout after spawn succeeds.
func TestLauncher_EnsureRunning_TimeoutWithMockSpawner(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-launcher-timeout-mock-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "m3"
	region := "r3"

	// Spawner that succeeds but doesn't start a server
	spawner := &mockSpawner{
		spawnFunc: func(_, _ string) error {
			// Don't actually start anything - simulate daemon failing to start
			return nil
		},
	}

	l := NewLauncher(accountID, region, withSpawner(spawner))

	// Should timeout since no daemon is listening
	err = l.EnsureRunning()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daemon did not start within timeout")
}

// TestLauncher_startProcess_WithMockSpawner tests startProcess with mock spawner.
func TestLauncher_startProcess_WithMockSpawner(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		spawner := &mockSpawner{}
		l := NewLauncher(testAccountID, testRegion, withSpawner(spawner))

		err := l.startProcess()
		require.NoError(t, err)
		assert.Equal(t, int32(1), spawner.spawnCount.Load())
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		spawner := &mockSpawner{
			spawnFunc: func(_, _ string) error {
				return errors.New("failed to spawn")
			},
		}
		l := NewLauncher(testAccountID, testRegion, withSpawner(spawner))

		err := l.startProcess()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to spawn")
	})
}

// TestDefaultProcessSpawner_Spawn tests the default spawner.
func TestDefaultProcessSpawner_Spawn(t *testing.T) {
	t.Parallel()
	t.Run("spawns process successfully", func(t *testing.T) {
		t.Parallel()
		// The default spawner will call os.Executable() which returns the test binary.
		// Since the test binary doesn't have "stage agent start" command, it will exit
		// quickly, but the Spawn itself should succeed (process starts and releases).
		spawner := &defaultProcessSpawner{}
		err := spawner.Spawn(testAccountID, testRegion)
		require.NoError(t, err)
	})

	t.Run("launcher uses default spawner", func(t *testing.T) {
		t.Parallel()

		l := NewLauncher(testAccountID, testRegion)
		require.NotNil(t, l.spawner)
		// Verify it's the default spawner type
		_, ok := l.spawner.(*defaultProcessSpawner)
		assert.True(t, ok)
	})
}

// TestWithSpawner tests the withSpawner option.
func TestWithSpawner(t *testing.T) {
	t.Parallel()

	spawner := &mockSpawner{}
	l := NewLauncher(testAccountID, testRegion, withSpawner(spawner))

	require.Same(t, spawner, l.spawner)
}
