package daemon

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

// testProcessScope1 and testProcessScope2 are used in tests that need to verify
// a single daemon can handle multiple scopes.
//
//nolint:gochecknoglobals // Test-only constants
var (
	testProcessScope1 = staging.AWSScope("111111111111", "us-east-1")
	testProcessScope2 = staging.AWSScope("222222222222", "us-west-2")
)

// TestDaemonProcess_StartupAndShutdown tests daemon startup and shutdown.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonProcess_StartupAndShutdown(t *testing.T) {
	// Create temp directory for socket (use /tmp to keep path short on macOS)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	// Create daemon with auto-shutdown disabled for controlled testing
	runner := NewRunner(WithAutoShutdownDisabled())

	// Start in background
	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(t.Context())
	}()

	// Wait for daemon to be ready
	launcher := NewLauncher(WithAutoStartDisabled())
	deadline := time.Now().Add(5 * time.Second)

	var ready bool

	for time.Now().Before(deadline) {
		if err := launcher.Ping(t.Context()); err == nil {
			ready = true

			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	require.True(t, ready, "daemon should be ready within timeout")

	// Verify socket file exists
	socketPath := protocol.SocketPath()
	_, statErr := os.Stat(socketPath)
	require.NoError(t, statErr, "socket file should exist")

	// Shutdown
	runner.Shutdown()

	// Wait for daemon to exit
	select {
	case err := <-errCh:
		// Should exit without error
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down within timeout")
	}
}

// TestDaemonProcess_MultipleScopes tests that a single daemon can handle multiple scopes.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonProcess_MultipleScopes(t *testing.T) {
	// Create temp directory for socket (use /tmp to keep path short on macOS)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-multi-scope-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	// Start a single daemon
	runner := NewRunner(WithAutoShutdownDisabled())
	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(t.Context())
	}()

	// Wait for daemon to be ready
	launcher := NewLauncher(WithAutoStartDisabled())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(t.Context()); err == nil {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	require.NoError(t, launcher.Ping(t.Context()), "daemon should be ready")

	// Stage entries for two different scopes
	stageReq1 := &protocol.Request{
		Method:  protocol.MethodStageEntry,
		Scope:   testProcessScope1,
		Service: staging.ServiceParam,
		Name:    "/scope1/param",
		Entry: &staging.Entry{
			Value:     lo.ToPtr("value1"),
			Operation: staging.OperationCreate,
		},
	}
	resp, err := launcher.SendRequest(t.Context(), stageReq1)
	require.NoError(t, err)
	require.True(t, resp.Success)

	stageReq2 := &protocol.Request{
		Method:  protocol.MethodStageEntry,
		Scope:   testProcessScope2,
		Service: staging.ServiceParam,
		Name:    "/scope2/param",
		Entry: &staging.Entry{
			Value:     lo.ToPtr("value2"),
			Operation: staging.OperationCreate,
		},
	}
	resp, err = launcher.SendRequest(t.Context(), stageReq2)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Verify both entries can be retrieved
	getReq1 := &protocol.Request{
		Method:  protocol.MethodGetEntry,
		Scope:   testProcessScope1,
		Service: staging.ServiceParam,
		Name:    "/scope1/param",
	}
	resp, err = launcher.SendRequest(t.Context(), getReq1)
	require.NoError(t, err)
	require.True(t, resp.Success)

	var result1 protocol.EntryResponse

	err = json.Unmarshal(resp.Data, &result1)
	require.NoError(t, err)
	require.NotNil(t, result1.Entry)
	assert.Equal(t, "value1", *result1.Entry.Value)

	getReq2 := &protocol.Request{
		Method:  protocol.MethodGetEntry,
		Scope:   testProcessScope2,
		Service: staging.ServiceParam,
		Name:    "/scope2/param",
	}
	resp, err = launcher.SendRequest(t.Context(), getReq2)
	require.NoError(t, err)
	require.True(t, resp.Success)

	var result2 protocol.EntryResponse

	err = json.Unmarshal(resp.Data, &result2)
	require.NoError(t, err)
	require.NotNil(t, result2.Entry)
	assert.Equal(t, "value2", *result2.Entry.Value)

	// Verify scope isolation - entry from scope1 should not exist in scope2
	getReqWrong := &protocol.Request{
		Method:  protocol.MethodGetEntry,
		Scope:   testProcessScope2,
		Service: staging.ServiceParam,
		Name:    "/scope1/param",
	}
	resp, err = launcher.SendRequest(t.Context(), getReqWrong)
	require.NoError(t, err)
	require.True(t, resp.Success)

	var resultWrong protocol.EntryResponse

	_ = json.Unmarshal(resp.Data, &resultWrong)
	assert.Nil(t, resultWrong.Entry, "entry from scope1 should not exist in scope2")

	// Cleanup
	runner.Shutdown()

	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
	}
}

// TestDaemonProcess_AutoShutdown tests automatic shutdown when state becomes empty.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonProcess_AutoShutdown(t *testing.T) {
	// Create temp directory for socket (use /tmp to keep path short on macOS)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-auto-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	scope := staging.AWSScope("a3", "r3")

	// Create daemon WITHOUT disabling auto-shutdown
	runner := NewRunner()

	// Start in background
	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(t.Context())
	}()

	// Wait for daemon to be ready
	launcher := NewLauncher(WithAutoStartDisabled())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(t.Context()); err == nil {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Stage an entry
	stageReq := &protocol.Request{
		Method:  protocol.MethodStageEntry,
		Scope:   scope,
		Service: staging.ServiceParam,
		Name:    "/test/param",
		Entry: &staging.Entry{
			Value:     lo.ToPtr("test-value"),
			Operation: staging.OperationCreate,
		},
	}
	resp, err := launcher.SendRequest(t.Context(), stageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Unstage the entry - this should trigger auto-shutdown because state becomes empty
	unstageReq := &protocol.Request{
		Method:  protocol.MethodUnstageEntry,
		Scope:   scope,
		Service: staging.ServiceParam,
		Name:    "/test/param",
	}
	resp, err = launcher.SendRequest(t.Context(), unstageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Daemon should shut down automatically (with some delay for the goroutine)
	select {
	case err := <-errCh:
		// Should exit without error
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		// Force shutdown if auto-shutdown didn't work
		runner.Shutdown()
		t.Fatal("daemon did not auto-shutdown within timeout after state became empty")
	}
}

// TestDaemonProcess_ManualModeDisablesAutoShutdown tests that manual mode prevents auto-shutdown.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonProcess_ManualModeDisablesAutoShutdown(t *testing.T) {
	// Create temp directory for socket (use /tmp to keep path short on macOS)
	tmpDir, err := os.MkdirTemp("/tmp", "suve-man-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	scope := staging.AWSScope("a4", "r4")

	// Create daemon with auto-shutdown DISABLED (manual mode)
	runner := NewRunner(WithAutoShutdownDisabled())

	// Start in background
	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(t.Context())
	}()

	// Wait for daemon to be ready
	launcher := NewLauncher(WithAutoStartDisabled())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(t.Context()); err == nil {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Stage and unstage an entry
	stageReq := &protocol.Request{
		Method:  protocol.MethodStageEntry,
		Scope:   scope,
		Service: staging.ServiceParam,
		Name:    "/test/param",
		Entry: &staging.Entry{
			Value:     lo.ToPtr("test-value"),
			Operation: staging.OperationCreate,
		},
	}
	resp, err := launcher.SendRequest(t.Context(), stageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	unstageReq := &protocol.Request{
		Method:  protocol.MethodUnstageEntry,
		Scope:   scope,
		Service: staging.ServiceParam,
		Name:    "/test/param",
	}
	resp, err = launcher.SendRequest(t.Context(), unstageReq)
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
	require.NoError(t, launcher.Ping(t.Context()))

	// Manual shutdown
	runner.Shutdown()

	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
	}
}

// TestDaemonProcess_AutoShutdown_UnstageAll tests automatic shutdown after UnstageAll.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonProcess_AutoShutdown_UnstageAll(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "suve-unstageall-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	scope := staging.AWSScope("a5", "r5")

	runner := NewRunner()
	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(t.Context())
	}()

	launcher := NewLauncher(WithAutoStartDisabled())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(t.Context()); err == nil {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Stage entries for both services
	for _, svc := range []staging.Service{staging.ServiceParam, staging.ServiceSecret} {
		stageReq := &protocol.Request{
			Method:  protocol.MethodStageEntry,
			Scope:   scope,
			Service: svc,
			Name:    "/test/param",
			Entry: &staging.Entry{
				Value:     lo.ToPtr("test-value"),
				Operation: staging.OperationCreate,
			},
		}
		resp, err := launcher.SendRequest(t.Context(), stageReq)
		require.NoError(t, err)
		require.True(t, resp.Success)
	}

	// UnstageAll with empty service clears both services and triggers auto-shutdown
	unstageReq := &protocol.Request{
		Method:  protocol.MethodUnstageAll,
		Scope:   scope,
		Service: "", // Empty clears all services
	}
	resp, err := launcher.SendRequest(t.Context(), unstageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Daemon should auto-shutdown
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		runner.Shutdown()
		t.Fatal("daemon did not auto-shutdown after UnstageAll")
	}
}

// TestDaemonProcess_AutoShutdown_UnstageTag tests automatic shutdown after UnstageTag empties state.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonProcess_AutoShutdown_UnstageTag(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "suve-unstagetag-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	scope := staging.AWSScope("a6", "r6")

	runner := NewRunner()
	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(t.Context())
	}()

	launcher := NewLauncher(WithAutoStartDisabled())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(t.Context()); err == nil {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Stage only a tag (no entry)
	stageReq := &protocol.Request{
		Method:  protocol.MethodStageTag,
		Scope:   scope,
		Service: staging.ServiceParam,
		Name:    "/test/param",
		TagEntry: &staging.TagEntry{
			Add: map[string]string{"key": "value"},
		},
	}
	resp, err := launcher.SendRequest(t.Context(), stageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// UnstageTag should trigger auto-shutdown when state becomes empty
	unstageReq := &protocol.Request{
		Method:  protocol.MethodUnstageTag,
		Scope:   scope,
		Service: staging.ServiceParam,
		Name:    "/test/param",
	}
	resp, err = launcher.SendRequest(t.Context(), unstageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Daemon should auto-shutdown
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		runner.Shutdown()
		t.Fatal("daemon did not auto-shutdown after UnstageTag")
	}
}

// TestDaemonProcess_AutoShutdown_SetState tests automatic shutdown after SetState with empty state.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonProcess_AutoShutdown_SetState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "suve-setstate-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	scope := staging.AWSScope("a7", "r7")

	runner := NewRunner()
	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(t.Context())
	}()

	launcher := NewLauncher(WithAutoStartDisabled())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(t.Context()); err == nil {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Stage an entry
	stageReq := &protocol.Request{
		Method:  protocol.MethodStageEntry,
		Scope:   scope,
		Service: staging.ServiceParam,
		Name:    "/test/param",
		Entry: &staging.Entry{
			Value:     lo.ToPtr("test-value"),
			Operation: staging.OperationCreate,
		},
	}
	resp, err := launcher.SendRequest(t.Context(), stageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// SetState with empty state should trigger auto-shutdown
	setStateReq := &protocol.Request{
		Method: protocol.MethodSetState,
		Scope:  scope,
		State:  staging.NewEmptyState(),
	}
	resp, err = launcher.SendRequest(t.Context(), setStateReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Daemon should auto-shutdown
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		runner.Shutdown()
		t.Fatal("daemon did not auto-shutdown after SetState with empty state")
	}
}

// TestDaemonProcess_AutoShutdown_UnstageAllEmpty tests automatic shutdown when UnstageAll is called on empty state.
// Note: This test cannot run in parallel because it modifies TMPDIR.
func TestDaemonProcess_AutoShutdown_UnstageAllEmpty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "suve-empty-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	scope := staging.AWSScope("a8", "r8")

	runner := NewRunner()
	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(t.Context())
	}()

	launcher := NewLauncher(WithAutoStartDisabled())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := launcher.Ping(t.Context()); err == nil {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Don't stage anything - state is already empty
	// UnstageAll on empty state should still trigger auto-shutdown check
	unstageReq := &protocol.Request{
		Method:  protocol.MethodUnstageAll,
		Scope:   scope,
		Service: "", // Empty clears all services
	}
	resp, err := launcher.SendRequest(t.Context(), unstageReq)
	require.NoError(t, err)
	require.True(t, resp.Success)

	// Daemon should auto-shutdown (state was already empty)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		runner.Shutdown()
		t.Fatal("daemon did not auto-shutdown after UnstageAll on empty state")
	}
}

// TestDaemonProcess_SocketPath tests the socket path structure.
func TestDaemonProcess_SocketPath(t *testing.T) {
	t.Parallel()

	path := protocol.SocketPath()

	// Path should end with agent.sock
	assert.Contains(t, path, "agent.sock")
}
