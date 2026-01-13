//go:build !linux && !darwin && !windows

package protocol

// SocketPath returns the path for the daemon socket.
func SocketPath() string { return socketPathFallback() }
