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

	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return value
	}
	return string(formatted)
}

// IsJSON checks if a string is valid JSON.
func IsJSON(value string) bool {
	var data any
	return json.Unmarshal([]byte(value), &data) == nil
}
