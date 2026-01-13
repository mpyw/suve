//go:build darwin

package protocol

import (
	"os"
	"path/filepath"
)

// socketPathForAccount returns the path for the daemon socket on macOS for a specific account/region.
func socketPathForAccount(accountID, region string) string {
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		return filepath.Join(tmpdir, socketDirName, accountID, region, socketFileName)
	}

	return socketPathFallback(accountID, region)
}
