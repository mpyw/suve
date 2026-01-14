//go:build linux

package protocol

import (
	"os"
	"path/filepath"
)

// socketPathForAccount returns the path for the daemon socket on Linux for a specific account/region.
func socketPathForAccount(accountID, region string) string {
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, socketDirName, accountID, region, socketFileName)
	}

	return socketPathFallback(accountID, region)
}
