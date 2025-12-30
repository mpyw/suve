// Package jsonutil provides JSON formatting utilities.
package jsonutil

import (
	"encoding/json/jsontext"
)

// Format formats a JSON string with indentation.
// If the input is not valid JSON, returns the original string unchanged.
func Format(value string) string {
	v := jsontext.Value(value)
	if !v.IsValid() {
		return value
	}
	if err := v.Indent(jsontext.WithIndent("  ")); err != nil {
		return value
	}
	return string(v)
}
