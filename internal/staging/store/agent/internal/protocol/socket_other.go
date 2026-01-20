//go:build !linux && !darwin && !windows

package protocol

// socketPath returns the path for the daemon socket.
func socketPath() string {
	return socketPathFallback()
}
