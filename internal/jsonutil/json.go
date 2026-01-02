// Package jsonutil provides JSON formatting utilities.
//
// The package supports pretty printing with indentation. Keys are always sorted
// alphabetically by Go's json.Marshal, ensuring consistent output for diffing.
package jsonutil

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mpyw/suve/internal/cli/colors"
)

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

// TryFormatOrWarn formats JSON or warns and returns original.
// If name is non-empty, includes it in the warning message.
func TryFormatOrWarn(value string, errW io.Writer, name string) string {
	if formatted, ok := TryFormat(value); ok {
		return formatted
	}
	warn(errW, name, "value is not valid JSON")
	return value
}

// TryFormatOrWarn2 formats both JSONs or warns and returns originals.
// If name is non-empty, includes it in the warning message.
func TryFormatOrWarn2(v1, v2 string, errW io.Writer, name string) (string, string) {
	f1, ok1 := TryFormat(v1)
	f2, ok2 := TryFormat(v2)
	if ok1 && ok2 {
		return f1, f2
	}
	warn(errW, name, "some values are not valid JSON")
	return v1, v2
}

func warn(w io.Writer, name, reason string) {
	msg := "--json has no effect"
	if name != "" {
		msg += " for " + name
	}
	msg += ": " + reason
	_, _ = fmt.Fprintln(w, colors.Warning("Warning: "+msg))
}
