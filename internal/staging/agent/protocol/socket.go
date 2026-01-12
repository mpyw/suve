package protocol

import (
	"fmt"
	"os"
	"path/filepath"
)

// socketPathFallback returns the fallback socket path.
func socketPathFallback() string {
	return filepath.Join(fmt.Sprintf("/tmp/suve-%d", os.Getuid()), "agent.sock")
}
