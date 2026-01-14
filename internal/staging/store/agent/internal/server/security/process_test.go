//go:build darwin || linux

package security_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/server/security"
)

//nolint:paralleltest // Intentionally not parallel: modifies global process state
func TestSetupProcess(t *testing.T) {
	// Note: Don't run in parallel as this modifies global process state.
	// This test actually modifies process state (disables core dumps).
	// We can't easily verify the effect without triggering a crash,
	// but we can at least verify it doesn't error.
	err := security.SetupProcess()
	require.NoError(t, err)

	// Calling it again should also succeed (idempotent)
	err = security.SetupProcess()
	require.NoError(t, err)
}
