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
	"github.com/aymanbagabas/go-udiff/myers"
	"github.com/fatih/color"
)

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
	cyan := color.New(color.FgCyan).SprintFunc()
	_, _ = fmt.Fprintf(o.w, "%s %s\n", cyan(label+":"), value)
}

// Separator prints a separator line.
func (o *Writer) Separator() {
	_, _ = fmt.Fprintln(o.w)
}

// Value prints a value with proper indentation.
func (o *Writer) Value(value string) {
	lines := strings.Split(value, "\n")
	for _, line := range lines {
		_, _ = fmt.Fprintf(o.w, "  %s\n", line)
	}
}

// Warning prints a warning message in yellow.
// Used to alert users about non-critical issues that don't prevent command execution.
// Example: "Warning: comparing identical versions"
func Warning(w io.Writer, format string, args ...any) {
	yellow := color.New(color.FgYellow).SprintFunc()
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, yellow("Warning: "+msg))
}

// Hint prints a hint message in cyan.
// Used to provide helpful suggestions to the user, typically following a warning.
// Example: "Hint: To compare with previous version, use: suve ssm diff /param~1"
func Hint(w io.Writer, format string, args ...any) {
	cyan := color.New(color.FgCyan).SprintFunc()
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, cyan("Hint: "+msg))
}

// Error prints an error message in red.
// Used for user-facing error messages that are not Go errors.
// For Go errors, use the standard error return pattern instead.
func Error(w io.Writer, format string, args ...any) {
	red := color.New(color.FgRed).SprintFunc()
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, red("Error: "+msg))
}

// Success prints a success message with green checkmark.
// Example: "✓ Set /app/config"
func Success(w io.Writer, format string, args ...any) {
	green := color.New(color.FgGreen).SprintFunc()
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(w, "%s %s\n", green("✓"), msg)
}

// Failed prints a failure message in red.
// Example: "Failed /app/config: error message"
func Failed(w io.Writer, name string, err error) {
	red := color.New(color.FgRed).SprintFunc()
	_, _ = fmt.Fprintf(w, "%s %s: %v\n", red("Failed"), name, err)
}

// Info prints an informational message in yellow (without "Warning:" prefix).
// Example: "No changes staged."
func Info(w io.Writer, format string, args ...any) {
	yellow := color.New(color.FgYellow).SprintFunc()
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, yellow(msg))
}

// Diff generates a unified diff between two strings.
func Diff(oldName, newName, oldContent, newContent string) string {
	edits := myers.ComputeEdits(oldContent, newContent)
	unified, _ := udiff.ToUnifiedDiff(oldName, newName, oldContent, edits)
	return colorDiff(unified.String())
}

// colorDiff adds ANSI colors to diff output.
func colorDiff(diff string) string {
	if diff == "" {
		return ""
	}

	lines := strings.Split(diff, "\n")
	var result strings.Builder

	red := color.New(color.FgRed).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			result.WriteString(cyan(line))
		case strings.HasPrefix(line, "-"):
			result.WriteString(red(line))
		case strings.HasPrefix(line, "+"):
			result.WriteString(green(line))
		case strings.HasPrefix(line, "@@"):
			result.WriteString(cyan(line))
		default:
			result.WriteString(line)
		}
		result.WriteString("\n")
	}

	return result.String()
}
