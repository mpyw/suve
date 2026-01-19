package protocol

const (
	// socketDirName is the directory name for the socket.
	socketDirName = "suve"
	// socketFileName is the socket file name.
	socketFileName = "agent.sock"
)

// SocketPath returns the socket path for the agent daemon.
// A single daemon handles all scopes, so the path is scope-independent.
func SocketPath() string {
	return socketPath()
}
