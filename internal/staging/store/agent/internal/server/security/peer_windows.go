//go:build windows

package security

import "net"

// VerifyPeerCredentials is a no-op on Windows.
//
// Windows AF_UNIX sockets do not support peer credential retrieval like
// Linux (SO_PEERCRED) or macOS (LOCAL_PEERCRED). Security relies on:
//
//  1. Socket file location in user's home directory (%USERPROFILE%\.suve)
//  2. Default Windows ACLs that restrict access to the owning user
//  3. Socket file created with restrictive permissions
//
// This provides equivalent security to Unix platforms where only the same
// user can connect to the daemon socket.
func VerifyPeerCredentials(_ net.Conn) error {
	return nil
}
