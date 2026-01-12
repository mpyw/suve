//go:build darwin

package security

// TODO: Implement macOS-specific process security using setrlimit(RLIMIT_CORE, 0)
// to disable core dumps and prevent secrets from being dumped to disk.

// SetupProcess is a no-op on macOS (not yet implemented).
func SetupProcess() error { return nil }
