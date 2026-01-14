// Package tagging provides unified tag management for SSM Parameter Store and Secrets Manager.
package tagging

import "github.com/mpyw/suve/internal/maputil"

// Change represents tag modifications to apply.
type Change struct {
	Add    map[string]string   // Tags to add or update
	Remove maputil.Set[string] // Tag keys to remove
}

// IsEmpty returns true if no changes are specified.
func (c *Change) IsEmpty() bool {
	return len(c.Add) == 0 && c.Remove.Len() == 0
}
