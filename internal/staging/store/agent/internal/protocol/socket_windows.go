//go:build windows

package protocol

import (
	"os"
	"path/filepath"
)

// SocketPath returns the path for the daemon socket on Windows.
func SocketPath() string {
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		return filepath.Join(localAppData, socketDirName, socketFileName)
	}
	// Fallback to user's home directory
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "."+socketDirName, socketFileName)
	}
	// Last resort: use temp directory
	return filepath.Join(os.TempDir(), socketDirName, socketFileName)
}
