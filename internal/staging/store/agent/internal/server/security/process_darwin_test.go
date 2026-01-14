//go:build darwin

package security_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/mpyw/suve/internal/staging/store/agent/internal/server/security"
)

//nolint:paralleltest // Intentionally not parallel: modifies global process state
func TestSetupProcess_Darwin(t *testing.T) {
	err := security.SetupProcess()
	require.NoError(t, err)

	// Verify core dumps are disabled by checking RLIMIT_CORE
	var rlim unix.Rlimit

	err = unix.Getrlimit(unix.RLIMIT_CORE, &rlim)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), rlim.Cur, "RLIMIT_CORE soft limit should be 0")
	assert.Equal(t, uint64(0), rlim.Max, "RLIMIT_CORE hard limit should be 0")
}
