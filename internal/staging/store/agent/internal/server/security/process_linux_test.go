//go:build linux

package security_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/server/security"
)

//nolint:paralleltest // Intentionally not parallel: modifies global process state
func TestSetupProcess_Linux(t *testing.T) {
	err := security.SetupProcess()
	require.NoError(t, err)

	// Verify core dumps are disabled via prctl.
	// The dumpable flag is returned in the first return value (not via pointer).
	dumpable, _, errno := unix.Syscall(unix.SYS_PRCTL, unix.PR_GET_DUMPABLE, 0, 0)
	require.Zero(t, errno, "prctl(PR_GET_DUMPABLE) should not fail")
	assert.Equal(t, uintptr(0), dumpable, "PR_GET_DUMPABLE should return 0 (not dumpable)")
}
