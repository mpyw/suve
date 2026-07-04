// Package secretversion provides version spec parsing and display helpers for
// AWS Secrets Manager.
package secretversion

// TruncateVersionID truncates a version ID to 8 characters for display.
// Secrets Manager version IDs are UUIDs which are long; this provides
// a readable short form similar to git commit hashes.
// versionIDDisplayLength is the number of characters to display for version IDs.
const versionIDDisplayLength = 8

// TruncateVersionID truncates a version ID to a readable short form.
func TruncateVersionID(id string) string {
	if len(id) > versionIDDisplayLength {
		return id[:versionIDDisplayLength]
	}

	return id
}
