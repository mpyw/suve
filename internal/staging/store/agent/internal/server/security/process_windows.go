//go:build windows

package security

// TODO: Implement Windows-specific process security to prevent minidumps
// from capturing sensitive data in memory.

// SetupProcess is a no-op on Windows (not yet implemented).
func SetupProcess() error { return nil }
