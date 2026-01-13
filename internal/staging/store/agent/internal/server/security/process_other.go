//go:build !linux && !darwin && !windows

package security

// SetupProcess is a no-op on unsupported platforms.
func SetupProcess() error {
	return nil
}
