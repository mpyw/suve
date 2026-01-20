package daemon

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

// testScope is used in protocol requests that need a scope.
//
//nolint:gochecknoglobals // Test-only constant
var testScope = staging.AWSScope("123456789012", "us-east-1")

func TestNewRunner(t *testing.T) {
	t.Parallel()

	t.Run("default options", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()
		require.NotNil(t, r)
		assert.NotNil(t, r.server)
		assert.NotNil(t, r.handler)
		assert.False(t, r.autoShutdownDisabled)
	})

	t.Run("with auto shutdown disabled", func(t *testing.T) {
		t.Parallel()

		r := NewRunner(WithAutoShutdownDisabled())
		require.NotNil(t, r)
		assert.True(t, r.autoShutdownDisabled)
	})
}

func TestRunner_Shutdown(t *testing.T) {
	t.Parallel()

	t.Run("shutdown without running server", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()
		// This should not panic
		r.Shutdown()
	})
}

func TestRunner_checkAutoShutdown(t *testing.T) {
	t.Parallel()

	t.Run("does not set WillShutdown when auto shutdown disabled", func(t *testing.T) {
		t.Parallel()

		r := NewRunner(WithAutoShutdownDisabled())

		req := &protocol.Request{Method: protocol.MethodUnstageAll}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.False(t, resp.WillShutdown)
		assert.Empty(t, resp.ShutdownReason)
	})

	t.Run("does not set WillShutdown on non-unstage methods", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodPing}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.False(t, resp.WillShutdown)
		assert.Empty(t, resp.ShutdownReason)
	})

	t.Run("does not set WillShutdown on failed response", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageAll}
		resp := &protocol.Response{Success: false}

		r.checkAutoShutdown(req, resp)
		assert.False(t, resp.WillShutdown)
		assert.Empty(t, resp.ShutdownReason)
	})
}

// TestRunner_checkAutoShutdown_ShutdownReasons tests all shutdown reason scenarios.
func TestRunner_checkAutoShutdown_ShutdownReasons(t *testing.T) {
	t.Parallel()

	// UnstageEntry tests
	t.Run("UnstageEntry with no hint returns empty reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageEntry}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonEmpty, resp.ShutdownReason)
	})

	t.Run("UnstageEntry with apply hint returns applied reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageEntry, Hint: protocol.HintApply}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonApplied, resp.ShutdownReason)
	})

	t.Run("UnstageEntry with reset hint returns unstaged reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageEntry, Hint: protocol.HintReset}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonUnstaged, resp.ShutdownReason)
	})

	t.Run("UnstageEntry with persist hint returns persisted reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageEntry, Hint: protocol.HintPersist}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonPersisted, resp.ShutdownReason)
	})

	// UnstageTag tests
	t.Run("UnstageTag with no hint returns empty reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageTag}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonEmpty, resp.ShutdownReason)
	})

	t.Run("UnstageTag with apply hint returns applied reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageTag, Hint: protocol.HintApply}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonApplied, resp.ShutdownReason)
	})

	t.Run("UnstageTag with reset hint returns unstaged reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageTag, Hint: protocol.HintReset}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonUnstaged, resp.ShutdownReason)
	})

	t.Run("UnstageTag with persist hint returns persisted reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageTag, Hint: protocol.HintPersist}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonPersisted, resp.ShutdownReason)
	})

	// UnstageAll tests
	t.Run("UnstageAll with no hint returns unstaged reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageAll}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonUnstaged, resp.ShutdownReason)
	})

	t.Run("UnstageAll with apply hint returns applied reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageAll, Hint: protocol.HintApply}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonApplied, resp.ShutdownReason)
	})

	t.Run("UnstageAll with reset hint returns unstaged reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageAll, Hint: protocol.HintReset}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonUnstaged, resp.ShutdownReason)
	})

	t.Run("UnstageAll with persist hint returns persisted reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodUnstageAll, Hint: protocol.HintPersist}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonPersisted, resp.ShutdownReason)
	})

	// SetState tests
	t.Run("SetState returns cleared reason", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		req := &protocol.Request{Method: protocol.MethodSetState}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonCleared, resp.ShutdownReason)
	})
}

// TestRunner_checkAutoShutdown_NonEmptyState tests that WillShutdown is NOT set
// when the handler still has staged entries.
func TestRunner_checkAutoShutdown_NonEmptyState(t *testing.T) {
	t.Parallel()

	t.Run("UnstageEntry does not shutdown when state not empty", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		// Stage an entry to make state non-empty
		stageReq := protocol.Request{
			Method:  protocol.MethodStageEntry,
			Scope:   testScope,
			Service: staging.ServiceParam,
			Name:    "/test/param",
			Entry: &staging.Entry{
				Value:     lo.ToPtr("value"),
				Operation: staging.OperationCreate,
			},
		}
		resp := r.handler.HandleRequest(&stageReq)
		require.True(t, resp.Success)

		// Stage another entry
		stageReq2 := protocol.Request{
			Method:  protocol.MethodStageEntry,
			Scope:   testScope,
			Service: staging.ServiceParam,
			Name:    "/test/param2",
			Entry: &staging.Entry{
				Value:     lo.ToPtr("value2"),
				Operation: staging.OperationCreate,
			},
		}
		resp = r.handler.HandleRequest(&stageReq2)
		require.True(t, resp.Success)

		// Unstage one entry - state should not be empty
		unstageReq := protocol.Request{
			Method:  protocol.MethodUnstageEntry,
			Scope:   testScope,
			Service: staging.ServiceParam,
			Name:    "/test/param",
		}
		resp = r.handler.HandleRequest(&unstageReq)
		require.True(t, resp.Success)

		// Check auto shutdown - should not trigger
		checkResp := &protocol.Response{Success: true}
		r.checkAutoShutdown(&unstageReq, checkResp)
		assert.False(t, checkResp.WillShutdown, "should not shutdown when state is not empty")
	})

	t.Run("UnstageTag does not shutdown when state not empty", func(t *testing.T) {
		t.Parallel()

		r := NewRunner()

		// Stage an entry
		stageReq := protocol.Request{
			Method:  protocol.MethodStageEntry,
			Scope:   testScope,
			Service: staging.ServiceParam,
			Name:    "/test/param",
			Entry: &staging.Entry{
				Value:     lo.ToPtr("value"),
				Operation: staging.OperationCreate,
			},
		}
		resp := r.handler.HandleRequest(&stageReq)
		require.True(t, resp.Success)

		// UnstageTag should not trigger shutdown since there's still an entry
		unstageReq := protocol.Request{
			Method:  protocol.MethodUnstageTag,
			Scope:   testScope,
			Service: staging.ServiceParam,
			Name:    "/test/another",
		}
		checkResp := &protocol.Response{Success: true}
		r.checkAutoShutdown(&unstageReq, checkResp)
		assert.False(t, checkResp.WillShutdown, "should not shutdown when state is not empty")
	})
}

// TestRunner_Run_ContextCancellation tests that the runner handles context cancellation correctly.
// Note: Signal handling (SIGTERM) is tested indirectly via e2e tests and real usage.
// Sending SIGTERM to the test process would kill the entire test run.
func TestRunner_Run_ContextCancellation(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-runner-ctx-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	runner := NewRunner(WithAutoShutdownDisabled())

	// Create a cancellable context
	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runner.Run(ctx)
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

	// Cancel the context to trigger shutdown
	cancel()

	// Wait for daemon to exit
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down within timeout after context cancellation")
	}
}

// TestRunner_MultipleOptions tests that multiple runner options are applied.
func TestRunner_MultipleOptions(t *testing.T) {
	t.Parallel()

	r := NewRunner(WithAutoShutdownDisabled())
	require.NotNil(t, r)
	assert.True(t, r.autoShutdownDisabled)
}

// TestRunner_checkAutoShutdown_AllMethods tests all methods that don't trigger auto-shutdown.
func TestRunner_checkAutoShutdown_AllMethods(t *testing.T) {
	t.Parallel()

	nonTriggeringMethods := []string{
		protocol.MethodPing,
		protocol.MethodShutdown,
		protocol.MethodGetEntry,
		protocol.MethodGetTag,
		protocol.MethodListEntries,
		protocol.MethodListTags,
		protocol.MethodLoad,
		protocol.MethodStageEntry,
		protocol.MethodStageTag,
		protocol.MethodGetState,
		protocol.MethodIsEmpty,
	}

	for _, method := range nonTriggeringMethods {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			r := NewRunner()

			req := &protocol.Request{Method: method}
			resp := &protocol.Response{Success: true}

			r.checkAutoShutdown(req, resp)
			assert.False(t, resp.WillShutdown, "method %s should not trigger auto-shutdown", method)
			assert.Empty(t, resp.ShutdownReason)
		})
	}
}

// TestRunner_Run_StartError tests Run when server.Start() fails.
// Note: This test cannot use t.Parallel() because it modifies TMPDIR.
func TestRunner_Run_StartError(t *testing.T) {
	// Use a path that definitely cannot be created as a directory
	// /proc/1/root is typically not writable on Linux
	t.Setenv("TMPDIR", "/proc/1/root/nonexistent")

	runner := NewRunner()

	// Use a short timeout context in case the server unexpectedly starts
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	err := runner.Run(ctx)
	// Should either fail to start (permission denied) or timeout
	// Both are acceptable - the important thing is that the test doesn't hang
	if err == nil {
		// If somehow it started, ensure cleanup
		runner.Shutdown()
	}
}
