package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAccountID = "123456789012"
	testRegion    = "us-east-1"
)

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
