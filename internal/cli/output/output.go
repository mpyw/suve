// Package output handles formatted output for the CLI.
//
// This package provides utilities for:
//   - Structured field output (label: value format)
//   - Unified diff generation with color highlighting
//   - User feedback messages (Warning, Hint, Error) with TTY-aware coloring
//
// Colors are automatically disabled when output is not a TTY, ensuring
// clean output when piped or redirected.
package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/aymanbagabas/go-udiff"

	"github.com/mpyw/suve/internal/cli/colors"
)

// Format represents the output format.
type Format string

const (
	// FormatText is the default human-readable text format.
	FormatText Format = "text"
	// FormatJSON outputs structured JSON.
	FormatJSON Format = "json"
)

// ParseFormat parses a format string and returns the Format.
// Returns FormatText for empty string or invalid values.
func ParseFormat(s string) Format {
	switch s {
	case "json":
		return FormatJSON
	default:
		return FormatText
	}
}

// Writer provides formatted output methods.
type Writer struct {
	w io.Writer
}

// New creates a new output writer.
func New(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Field prints a labeled field.
func (o *Writer) Field(label, value string) {
	_, _ = fmt.Fprintf(o.w, "%s %s\n", colors.FieldLabel(label+":"), value)
}

// Separator prints a separator line.
func (o *Writer) Separator() {
	_, _ = fmt.Fprintln(o.w)
}

// Value prints a value with proper indentation.
func (o *Writer) Value(value string) {
	for line := range strings.SplitSeq(value, "\n") {
		_, _ = fmt.Fprintf(o.w, "  %s\n", line)
	}
}

// Warning prints a warning message in yellow.
// Used to alert users about non-critical issues that don't prevent command execution.
// Example: "Warning: comparing identical versions".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Warning(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, colors.Warning("Warning: "+msg))
}

// Hint prints a hint message in cyan.
// Used to provide helpful suggestions to the user, typically following a warning.
// Example: "Hint: To compare with previous version, use: suve param diff /param~1".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Hint(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, colors.Info("Hint: "+msg))
}

// Error prints an error message in red.
// Used for user-facing error messages that are not Go errors.
// For Go errors, use the standard error return pattern instead.
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Error(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, colors.Error("Error: "+msg))
}

// Success prints a success message with green checkmark.
// Example: "✓ Created parameter /app/config".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Success(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(w, "%s %s\n", colors.Success("✓"), msg)
}

// Failed prints a failure message in red.
// Example: "Failed /app/config: error message".
func Failed(w io.Writer, name string, err error) {
	_, _ = fmt.Fprintf(w, "%s %s: %v\n", colors.Failed("Failed"), name, err)
}

// Info prints an informational message in yellow (without "Warning:" prefix).
// Example: "No changes staged.".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Info(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, colors.Warning(msg))
}

// Warn prints a warning message with yellow "!" prefix.
// Example: "! Skipped /app/config (same as AWS)".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Warn(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(w, "%s %s\n", colors.Warning("!"), msg)
}

// Diff generates a unified diff between two strings with ANSI colors.
func Diff(oldName, newName, oldContent, newContent string) string {
	edits := udiff.Strings(oldContent, newContent)
	unified, _ := udiff.ToUnifiedDiff(oldName, newName, oldContent, edits, udiff.DefaultContextLines)

	return colorDiff(unified.String())
}

// DiffRaw generates a unified diff between two strings without colors.
func DiffRaw(oldName, newName, oldContent, newContent string) string {
	edits := udiff.Strings(oldContent, newContent)
	unified, _ := udiff.ToUnifiedDiff(oldName, newName, oldContent, edits, udiff.DefaultContextLines)

	return unified.String()
}

// colorDiff adds ANSI colors to diff output.
func colorDiff(diff string) string {
	if diff == "" {
		return ""
	}

	lines := strings.Split(diff, "\n")

	var result strings.Builder

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			result.WriteString(colors.DiffHeader(line))
		case strings.HasPrefix(line, "-"):
			result.WriteString(colors.DiffRemoved(line))
		case strings.HasPrefix(line, "+"):
			result.WriteString(colors.DiffAdded(line))
		case strings.HasPrefix(line, "@@"):
			result.WriteString(colors.DiffHunk(line))
		default:
			result.WriteString(line)
		}

		result.WriteString("\n")
	}

	return result.String()
}

// Print writes a message to the writer without a newline.
func Print(w io.Writer, msg string) {
	_, _ = fmt.Fprint(w, msg)
}

// Println writes a message to the writer with a newline.
func Println(w io.Writer, msg string) {
	_, _ = fmt.Fprintln(w, msg)
}

// Printf writes a formatted message to the writer.
func Printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// Indent adds a prefix to each line of the input string.
func Indent(s, prefix string) string {
	if s == "" {
		return ""
	}

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}

	return strings.Join(lines, "\n")
}
