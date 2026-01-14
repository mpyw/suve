//go:build !windows

package protocol

import (
	"fmt"
	"os"
	"path/filepath"
)

// socketPathFallback returns the fallback socket path for a specific account/region.
// Used by darwin, linux, and other Unix-like platforms when preferred paths are unavailable.
func socketPathFallback(accountID, region string) string {
	return filepath.Join(fmt.Sprintf("/tmp/%s-%d", socketDirName, os.Getuid()), accountID, region, socketFileName)
}
