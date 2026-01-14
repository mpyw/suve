//go:build darwin

package security

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// SetupProcess configures macOS-specific security measures.
func SetupProcess() error {
	// Disable core dumps to prevent secrets from being dumped to disk
	rlim := unix.Rlimit{
		Cur: 0,
		Max: 0,
	}

	if err := unix.Setrlimit(unix.RLIMIT_CORE, &rlim); err != nil {
		return fmt.Errorf("failed to disable core dumps: %w", err)
	}

	return nil
}
