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

func TestRunner_handleAutoShutdown(t *testing.T) {
	t.Parallel()

	t.Run("does nothing when auto shutdown disabled", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion, WithAutoShutdownDisabled())

		req := &protocol.Request{Method: protocol.MethodUnstageAll}
		resp := &protocol.Response{Success: true}

		// Should not panic or trigger shutdown
		r.handleAutoShutdown(req, resp)
	})

	t.Run("does nothing on non-unstage methods", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodPing}
		resp := &protocol.Response{Success: true}

		// Should not trigger shutdown
		r.handleAutoShutdown(req, resp)
		assert.True(t, r.handler.IsEmpty())
	})

	t.Run("does nothing on failed response", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(testRunnerAccountID, testRunnerRegion)

		req := &protocol.Request{Method: protocol.MethodUnstageAll}
		resp := &protocol.Response{Success: false}

		// Should not trigger shutdown because response failed
		r.handleAutoShutdown(req, resp)
	})
}
