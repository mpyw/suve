// Package jsonutil provides JSON formatting utilities.
//
// The package supports pretty printing with indentation. Keys are always sorted
// alphabetically by Go's json.Marshal, ensuring consistent output for diffing.
package jsonutil

import (
	"encoding/json"
)

// Format formats a JSON string with indentation.
// Keys are automatically sorted alphabetically by Go's json.Marshal.
// If the input is not valid JSON, returns the original string unchanged.
func Format(value string) string {
	var data any
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		return value
	}
	// MarshalIndent cannot fail here: data contains only JSON-safe types
	// (map, slice, string, number, bool, nil) after successful Unmarshal.
	formatted, _ := json.MarshalIndent(data, "", "  ")
	return string(formatted)
}

// IsJSON checks if a string is valid JSON.
func IsJSON(value string) bool {
	var data any
	return json.Unmarshal([]byte(value), &data) == nil
}

// TryFormat attempts to format a JSON string with indentation.
// Returns the formatted string and true if successful, or the original string and false if not valid JSON.
// This is useful for commands that need to know whether formatting was applied.
func TryFormat(value string) (string, bool) {
	var data any
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		return value, false
	}
	formatted, _ := json.MarshalIndent(data, "", "  ")
	return string(formatted), true
}
