//go:build darwin

package protocol

import (
	"os"
	"path/filepath"
)

// socketPath returns the path for the daemon socket on macOS.
func socketPath() string {
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		return filepath.Join(tmpdir, socketDirName, socketFileName)
	}

	return socketPathFallback()
}
