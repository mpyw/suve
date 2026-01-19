//go:build !windows

package protocol

import (
	"fmt"
	"os"
	"path/filepath"
)

// socketPathFallback returns the fallback socket path.
// Used by darwin, linux, and other Unix-like platforms when preferred paths are unavailable.
func socketPathFallback() string {
	return filepath.Join(fmt.Sprintf("/tmp/%s-%d", socketDirName, os.Getuid()), socketFileName)
}
