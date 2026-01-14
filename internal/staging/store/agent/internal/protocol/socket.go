package protocol

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
