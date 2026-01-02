// Package tagging provides unified tag management for SSM Parameter Store and Secrets Manager.
package tagging

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
)

// Change represents tag modifications to apply.
type Change struct {
	Add    map[string]string // Tags to add or update
	Remove []string          // Tag keys to remove
}

// IsEmpty returns true if no changes are specified.
func (c *Change) IsEmpty() bool {
	return len(c.Add) == 0 && len(c.Remove) == 0
}

// ParseResult contains parsed tag changes and any warnings.
type ParseResult struct {
	Change   *Change
	Warnings []string
}

// ParseFlags parses --tag and --untag flag values.
// Returns warnings if the same key appears in both (later wins).
//
// Tag format: "key=value"
// Untag format: "key"
func ParseFlags(tags []string, untags []string) (*ParseResult, error) {
	result := &ParseResult{
		Change: &Change{
			Add:    make(map[string]string),
			Remove: make([]string, 0),
		},
		Warnings: make([]string, 0),
	}

	// Track order of operations for conflict resolution
	type operation struct {
		isAdd bool
		key   string
		value string // only for add
		index int
	}
	var ops []operation
	idx := 0

	// Parse --tag flags
	for _, tag := range tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format %q: expected key=value", tag)
		}
		key, value := parts[0], parts[1]
		if key == "" {
			return nil, fmt.Errorf("invalid tag format %q: key cannot be empty", tag)
		}
		ops = append(ops, operation{isAdd: true, key: key, value: value, index: idx})
		idx++
	}

	// Parse --untag flags
	for _, key := range untags {
		if key == "" {
			return nil, fmt.Errorf("invalid untag: key cannot be empty")
		}
		ops = append(ops, operation{isAdd: false, key: key, index: idx})
		idx++
	}

	// Process operations, detecting conflicts
	seen := make(map[string]operation)
	for _, op := range ops {
		if prev, exists := seen[op.key]; exists {
			// Conflict detected
			prevAction := lo.Ternary(prev.isAdd,
				fmt.Sprintf("--tag %s=%s", prev.key, prev.value),
				fmt.Sprintf("--untag %s", prev.key),
			)
			newAction := lo.Ternary(op.isAdd,
				fmt.Sprintf("--tag %s=%s", op.key, op.value),
				fmt.Sprintf("--untag %s", op.key),
			)
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("tag %q: %s overrides %s", op.key, newAction, prevAction))
		}
		seen[op.key] = op
	}

	// Build final change set from last operation per key
	for key, op := range seen {
		if op.isAdd {
			result.Change.Add[key] = op.value
		} else {
			result.Change.Remove = append(result.Change.Remove, key)
		}
	}

	return result, nil
}
