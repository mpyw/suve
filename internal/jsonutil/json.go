// Package jsonutil provides JSON formatting utilities.
//
// The package supports pretty printing with indentation. Keys are always sorted
// alphabetically by Go's json.Marshal, ensuring consistent output for diffing.
package jsonutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/mpyw/suve/internal/cli/output"
)

// TryFormat attempts to format a JSON string with indentation.
// Returns the formatted string and true if successful, or the original string and false if not valid JSON.
// This is useful for commands that need to know whether formatting was applied.
//
// HTML escaping is disabled so that characters like &, <, > survive verbatim
// instead of being mangled into &, <, > — secret/parameter values
// are not HTML and are never rendered in a browser context.
//
// Numbers are decoded with UseNumber so their exact literal is preserved:
// decoding into float64 (the default) silently loses integer precision beyond
// 2^53 and collapses 1.0/1, which would make genuinely different values format
// identically (and diff as "identical").
//
// Values carrying invalid UTF-8 are left unformatted: Go's json encoder coerces
// invalid bytes to the replacement character (U+FFFD), which would silently
// mutate the displayed value, so we fall back to the raw string instead.
func TryFormat(value string) (string, bool) {
	if !utf8.ValidString(value) {
		return value, false
	}

	dec := json.NewDecoder(bytes.NewReader([]byte(value)))
	dec.UseNumber()

	var data any
	if err := dec.Decode(&data); err != nil {
		return value, false
	}

	// Reject trailing data after the JSON value: json.Decoder stops at the end
	// of the first value, whereas json.Unmarshal treated trailing content as
	// invalid. A clean single-value input yields io.EOF here.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return value, false
	}

	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(data); err != nil {
		// This should not happen if Decode succeeded, but handle it gracefully
		return value, false
	}

	// json.Encoder.Encode appends a trailing newline; strip it to match the
	// previous json.MarshalIndent behavior.
	return string(bytes.TrimRight(buf.Bytes(), "\n")), true
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
	msg := "--parse-json has no effect"
	if name != "" {
		msg += " for " + name
	}

	msg += ": " + reason
	output.Warning(w, "%s", msg)
}
