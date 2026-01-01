// Package smutil provides shared utilities for Secrets Manager commands.
package smutil

// TruncateVersionID truncates a version ID to 8 characters for display.
// Secrets Manager version IDs are UUIDs which are long; this provides
// a readable short form similar to git commit hashes.
func TruncateVersionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
