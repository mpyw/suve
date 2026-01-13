package daemon

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/protocol"
)

const (
	testRunnerAccountID = "123456789012"
	testRunnerRegion    = "us-east-1"
)

func TestNewRunner(t *testing.T) {
	t.Parallel()

	t.Run("default options", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)
		require.NotNil(t, r)
		assert.NotNil(t, r.server)
		assert.NotNil(t, r.handler)
		assert.Equal(t, testRunnerAccountID, r.accountID)
		assert.Equal(t, testRunnerRegion, r.region)
		assert.False(t, r.autoShutdownDisabled)
	})

	t.Run("with auto shutdown disabled", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion, WithAutoShutdownDisabled())
		require.NotNil(t, r)
		assert.True(t, r.autoShutdownDisabled)
	})
}

func TestRunner_Shutdown(t *testing.T) {
	t.Parallel()

	t.Run("shutdown without running server", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)
		// This should not panic
		r.Shutdown()
	})
}

func TestRunner_checkAutoShutdown(t *testing.T) {
	t.Parallel()

	t.Run("does not set WillShutdown when auto shutdown disabled", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion, WithAutoShutdownDisabled())

		req := &protocol.Request{Method: protocol.MethodUnstageAll}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.False(t, resp.WillShutdown)
		assert.Empty(t, resp.ShutdownReason)
	})

	t.Run("does not set WillShutdown on non-unstage methods", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodPing}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.False(t, resp.WillShutdown)
		assert.Empty(t, resp.ShutdownReason)
	})

	t.Run("does not set WillShutdown on failed response", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

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
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageEntry}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonEmpty, resp.ShutdownReason)
	})

	t.Run("UnstageEntry with apply hint returns applied reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageEntry, Hint: protocol.HintApply}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonApplied, resp.ShutdownReason)
	})

	t.Run("UnstageEntry with reset hint returns unstaged reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageEntry, Hint: protocol.HintReset}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonUnstaged, resp.ShutdownReason)
	})

	t.Run("UnstageEntry with persist hint returns persisted reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageEntry, Hint: protocol.HintPersist}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonPersisted, resp.ShutdownReason)
	})

	// UnstageTag tests
	t.Run("UnstageTag with no hint returns empty reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageTag}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonEmpty, resp.ShutdownReason)
	})

	t.Run("UnstageTag with apply hint returns applied reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageTag, Hint: protocol.HintApply}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonApplied, resp.ShutdownReason)
	})

	t.Run("UnstageTag with reset hint returns unstaged reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageTag, Hint: protocol.HintReset}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonUnstaged, resp.ShutdownReason)
	})

	t.Run("UnstageTag with persist hint returns persisted reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageTag, Hint: protocol.HintPersist}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonPersisted, resp.ShutdownReason)
	})

	// UnstageAll tests
	t.Run("UnstageAll with no hint returns unstaged reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageAll}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonUnstaged, resp.ShutdownReason)
	})

	t.Run("UnstageAll with apply hint returns applied reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageAll, Hint: protocol.HintApply}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonApplied, resp.ShutdownReason)
	})

	t.Run("UnstageAll with reset hint returns unstaged reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageAll, Hint: protocol.HintReset}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonUnstaged, resp.ShutdownReason)
	})

	t.Run("UnstageAll with persist hint returns persisted reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageAll, Hint: protocol.HintPersist}
		resp := &protocol.Response{Success: true}

		r.checkAutoShutdown(req, resp)
		assert.True(t, resp.WillShutdown)
		assert.Equal(t, protocol.ShutdownReasonPersisted, resp.ShutdownReason)
	})

	// SetState tests
	t.Run("SetState returns cleared reason", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

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
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		// Stage an entry to make state non-empty
		stageReq := protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: testRunnerAccountID,
			Region:    testRunnerRegion,
			Service:   staging.ServiceParam,
			Name:      "/test/param",
			Entry: &staging.Entry{
				Value:     lo.ToPtr("value"),
				Operation: staging.OperationCreate,
			},
		}
		resp := r.handler.HandleRequest(&stageReq)
		require.True(t, resp.Success)

		// Stage another entry
		stageReq2 := protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: testRunnerAccountID,
			Region:    testRunnerRegion,
			Service:   staging.ServiceParam,
			Name:      "/test/param2",
			Entry: &staging.Entry{
				Value:     lo.ToPtr("value2"),
				Operation: staging.OperationCreate,
			},
		}
		resp = r.handler.HandleRequest(&stageReq2)
		require.True(t, resp.Success)

		// Unstage one entry - state should not be empty
		unstageReq := protocol.Request{
			Method:    protocol.MethodUnstageEntry,
			AccountID: testRunnerAccountID,
			Region:    testRunnerRegion,
			Service:   staging.ServiceParam,
			Name:      "/test/param",
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
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		// Stage an entry
		stageReq := protocol.Request{
			Method:    protocol.MethodStageEntry,
			AccountID: testRunnerAccountID,
			Region:    testRunnerRegion,
			Service:   staging.ServiceParam,
			Name:      "/test/param",
			Entry: &staging.Entry{
				Value:     lo.ToPtr("value"),
				Operation: staging.OperationCreate,
			},
		}
		resp := r.handler.HandleRequest(&stageReq)
		require.True(t, resp.Success)

		// UnstageTag should not trigger shutdown since there's still an entry
		unstageReq := protocol.Request{
			Method:    protocol.MethodUnstageTag,
			AccountID: testRunnerAccountID,
			Region:    testRunnerRegion,
			Service:   staging.ServiceParam,
			Name:      "/test/another",
		}
		checkResp := &protocol.Response{Success: true}
		r.checkAutoShutdown(&unstageReq, checkResp)
		assert.False(t, checkResp.WillShutdown, "should not shutdown when state is not empty")
	})
}

// TestRunner_Run_SignalHandling tests that the runner handles signals correctly.
func TestRunner_Run_SignalHandling(t *testing.T) {
	// Create temp directory for socket
	tmpDir, err := os.MkdirTemp("/tmp", "suve-runner-signal-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	t.Setenv("TMPDIR", tmpDir)

	accountID := "c1"
	region := "r1"

	runner := NewRunner(accountID, region, WithAutoShutdownDisabled())

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
	require.NoError(t, launcher.Ping(), "daemon should be ready")

	// Send SIGTERM to trigger shutdown
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	err = process.Signal(syscall.SIGTERM)
	require.NoError(t, err)

	// Wait for daemon to exit
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down within timeout after SIGTERM")
	}
}

// TestRunner_MultipleOptions tests that multiple runner options are applied.
func TestRunner_MultipleOptions(t *testing.T) {
	t.Parallel()

	r := NewRunner(testRunnerAccountID, testRunnerRegion,
		WithAutoShutdownDisabled(),
	)
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
			r := NewRunner(testRunnerAccountID, testRunnerRegion)

			req := &protocol.Request{Method: method}
			resp := &protocol.Response{Success: true}

			r.checkAutoShutdown(req, resp)
			assert.False(t, resp.WillShutdown, "method %s should not trigger auto-shutdown", method)
			assert.Empty(t, resp.ShutdownReason)
		})
	}
}

// TestRunner_Run_StartError tests Run when server.Start() fails.
func TestRunner_Run_StartError(t *testing.T) {
	// Set TMPDIR to an invalid path to cause Start to fail
	t.Setenv("TMPDIR", "/dev/null")

	accountID := "run-start-err"
	region := "r1"

	runner := NewRunner(accountID, region)

	err := runner.Run()
	require.Error(t, err)
	// Error should be from Start() failing to create socket directory
}
