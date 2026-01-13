package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

// TestDaemonLifecycle_AccountIsolation tests that daemons for different accounts are isolated.
func TestDaemonLifecycle_AccountIsolation(t *testing.T) {
	t.Parallel()

	account1 := "111111111111"
	account2 := "222222222222"
	region := "us-east-1"

	// Socket paths should be different for different accounts
	path1 := protocol.SocketPathForAccount(account1, region)
	path2 := protocol.SocketPathForAccount(account2, region)

	assert.NotEqual(t, path1, path2, "different accounts should have different socket paths")
	assert.Contains(t, path1, account1)
	assert.Contains(t, path2, account2)
}

// TestDaemonLifecycle_RegionIsolation tests that daemons for different regions are isolated.
func TestDaemonLifecycle_RegionIsolation(t *testing.T) {
	t.Parallel()

	account := "123456789012"
	region1 := "us-east-1"
	region2 := "us-west-2"

	// Socket paths should be different for different regions
	path1 := protocol.SocketPathForAccount(account, region1)
	path2 := protocol.SocketPathForAccount(account, region2)

	assert.NotEqual(t, path1, path2, "different regions should have different socket paths")
	assert.Contains(t, path1, region1)
	assert.Contains(t, path2, region2)
}

// TestDaemonLifecycle_StartupAndShutdown tests daemon startup and shutdown.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonLifecycle_StartupAndShutdown(t *testing.T) {
	// Create temp directory for socket (use /tmp to keep path short on macOS)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "a1"
	region := "r1"

	// Create daemon with auto-shutdown disabled for controlled testing
	runner := NewRunner(accountID, region, WithAutoShutdownDisabled())

	// Start in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run()
	}()

	// Wait for daemon to be ready
	launcher := NewLauncher(accountID, region, WithAutoStartDisabled())
	deadline := time.Now().Add(5 * time.Second)
	var ready bool
	for time.Now().Before(deadline) {
		if err := launcher.Ping(); err == nil {
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, ready, "daemon should be ready within timeout")

	// Verify socket file exists
	socketPath := protocol.SocketPathForAccount(accountID, region)
	_, statErr := os.Stat(socketPath)
	require.NoError(t, statErr, "socket file should exist")

	// Shutdown
	runner.Shutdown()

	// Wait for daemon to exit
	select {
	case err := <-errCh:
		// Should exit without error
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down within timeout")
	}
}

// TestDaemonLifecycle_MultipleAccountsSimultaneous tests running daemons for different accounts simultaneously.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonLifecycle_MultipleAccountsSimultaneous(t *testing.T) {
	// Create temp directory for socket (use /tmp to keep path short on macOS)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-multi-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	// Two different accounts
	account1 := "a1"
	account2 := "a2"
	region := "r1"

	// Start daemon for account1
	runner1 := NewRunner(account1, region, WithAutoShutdownDisabled())
	errCh1 := make(chan error, 1)
	go func() {
		errCh1 <- runner1.Run()
	}()

	// Start daemon for account2
	runner2 := NewRunner(account2, region, WithAutoShutdownDisabled())
	errCh2 := make(chan error, 1)
	go func() {
		errCh2 <- runner2.Run()
	}()

	// Wait for both daemons to be ready
	launcher1 := NewLauncher(account1, region, WithAutoStartDisabled())
	launcher2 := NewLauncher(account2, region, WithAutoStartDisabled())

	deadline := time.Now().Add(5 * time.Second)
	var ready1, ready2 bool
	for time.Now().Before(deadline) && (!ready1 || !ready2) {
		if !ready1 && launcher1.Ping() == nil {
			ready1 = true
		}
		if !ready2 && launcher2.Ping() == nil {
			ready2 = true
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, ready1, "daemon for account1 should be ready")
	require.True(t, ready2, "daemon for account2 should be ready")

	// Both should respond independently
	require.NoError(t, launcher1.Ping())
	require.NoError(t, launcher2.Ping())

	// Cleanup
	runner1.Shutdown()
	runner2.Shutdown()
	select {
	case <-errCh1:
	case <-time.After(5 * time.Second):
	}
	select {
	case <-errCh2:
	case <-time.After(5 * time.Second):
	}
}

// TestDaemonLifecycle_AutoShutdown tests automatic shutdown when state becomes empty.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonLifecycle_AutoShutdown(t *testing.T) {
	// Create temp directory for socket (use /tmp to keep path short on macOS)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-auto-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "a3"
	region := "r3"

	// Create daemon WITHOUT disabling auto-shutdown
	runner := NewRunner(accountID, region)

	// Start in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run()
	}()

	// Wait for daemon to be ready
	launcher := NewLauncher(accountID, region, WithAutoStartDisabled())
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Stage an entry
	stageReq := &protocol.Request{
		Method:    protocol.MethodStageEntry,
		AccountID: accountID,
		Region:    region,
		Service:   staging.ServiceParam,
		Name:      "/test/param",
		Entry: &staging.Entry{
			Value:     lo.ToPtr("test-value"),
			Operation: staging.OperationCreate,
		},
	}
	resp, err := launcher.SendRequest(stageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Unstage the entry - this should trigger auto-shutdown because state becomes empty
	unstageReq := &protocol.Request{
		Method:    protocol.MethodUnstageEntry,
		AccountID: accountID,
		Region:    region,
		Service:   staging.ServiceParam,
		Name:      "/test/param",
	}
	resp, err = launcher.SendRequest(unstageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Daemon should shut down automatically (with some delay for the goroutine)
	select {
	case err := <-errCh:
		// Should exit without error
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		// Force shutdown if auto-shutdown didn't work
		runner.Shutdown()
		t.Fatal("daemon did not auto-shutdown within timeout after state became empty")
	}
}

// TestDaemonLifecycle_ManualModeDisablesAutoShutdown tests that manual mode prevents auto-shutdown.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonLifecycle_ManualModeDisablesAutoShutdown(t *testing.T) {
	// Create temp directory for socket (use /tmp to keep path short on macOS)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-man-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "a4"
	region := "r4"

	// Create daemon with auto-shutdown DISABLED (manual mode)
	runner := NewRunner(accountID, region, WithAutoShutdownDisabled())

	// Start in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run()
	}()

	// Wait for daemon to be ready
	launcher := NewLauncher(accountID, region, WithAutoStartDisabled())
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Stage and unstage an entry
	stageReq := &protocol.Request{
		Method:    protocol.MethodStageEntry,
		AccountID: accountID,
		Region:    region,
		Service:   staging.ServiceParam,
		Name:      "/test/param",
		Entry: &staging.Entry{
			Value:     lo.ToPtr("test-value"),
			Operation: staging.OperationCreate,
		},
	}
	resp, err := launcher.SendRequest(stageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	unstageReq := &protocol.Request{
		Method:    protocol.MethodUnstageEntry,
		AccountID: accountID,
		Region:    region,
		Service:   staging.ServiceParam,
		Name:      "/test/param",
	}
	resp, err = launcher.SendRequest(unstageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Daemon should NOT shutdown automatically in manual mode
	// Wait a bit to make sure it doesn't shutdown
	select {
	case <-errCh:
		t.Fatal("daemon should not auto-shutdown in manual mode")
	case <-time.After(500 * time.Millisecond):
		// Good - daemon is still running
	}

	// Should still be able to ping
	require.NoError(t, launcher.Ping())

	// Manual shutdown
	runner.Shutdown()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
	}
}

// TestDaemonLifecycle_SocketPathStructure tests the socket path structure includes account and region.
func TestDaemonLifecycle_SocketPathStructure(t *testing.T) {
	t.Parallel()

	accountID := "123456789012"
	region := "ap-northeast-1"

	path := protocol.SocketPathForAccount(accountID, region)

	// Path should contain account ID and region as directory components
	assert.Contains(t, path, accountID)
	assert.Contains(t, path, region)
	assert.Contains(t, path, "agent.sock")

	// Path should have proper structure
	dir := filepath.Dir(path)
	assert.Contains(t, dir, accountID)
	assert.Contains(t, dir, region)
}
