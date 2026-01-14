//go:build linux

package security

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// SetupProcess configures Linux-specific security measures.
func SetupProcess() error {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("failed to disable core dumps: %w", err)
	}

	return nil
}
