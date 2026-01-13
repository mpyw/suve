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

// SocketPathForAccount returns the socket path for a specific AWS account and region.
// This ensures each account/region combination has its own daemon instance.
func SocketPathForAccount(accountID, region string) string {
	return socketPathForAccount(accountID, region)
}

// socketPathFallback returns the fallback socket path for a specific account/region.
func socketPathFallback(accountID, region string) string {
	return filepath.Join(fmt.Sprintf("/tmp/%s-%d", socketDirName, os.Getuid()), accountID, region, socketFileName)
}
