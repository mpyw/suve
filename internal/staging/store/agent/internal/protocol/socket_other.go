//go:build !linux && !darwin && !windows

package protocol

// socketPathForAccount returns the path for the daemon socket for a specific account/region.
func socketPathForAccount(accountID, region string) string {
	return socketPathFallback(accountID, region)
}
