//go:build windows

package protocol

import (
	"os"
	"path/filepath"
)

// socketPathForAccount returns the path for the daemon socket on Windows for a specific account/region.
func socketPathForAccount(accountID, region string) string {
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		return filepath.Join(localAppData, socketDirName, accountID, region, socketFileName)
	}

	// Fallback to user's home directory
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "."+socketDirName, accountID, region, socketFileName)
	}

	// Last resort: use temp directory
	return filepath.Join(os.TempDir(), socketDirName, accountID, region, socketFileName)
}
