package protocol

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// socketDirName is the directory name for the socket.
	socketDirName = "suve"
	// socketFileName is the socket file name.
	socketFileName = "agent.sock"
)

// socketPathFallback returns the fallback socket path.
func socketPathFallback() string {
	return filepath.Join(fmt.Sprintf("/tmp/%s-%d", socketDirName, os.Getuid()), socketFileName)
}
