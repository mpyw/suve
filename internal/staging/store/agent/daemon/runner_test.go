package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
