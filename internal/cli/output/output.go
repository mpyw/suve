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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aymanbagabas/go-udiff"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/terminal"
)

// colorEnabled reports whether ANSI color should be emitted for w: only when
// NO_COLOR is unset and w itself is a terminal. Deciding per destination writer
// — rather than from the process-global color.NoColor, which fatih/color derives
// once from os.Stdout — keeps stderr's coloring correct under redirection such
// as `suve … 2>err.log` (stderr not a TTY) or `suve … | cat` (stderr a TTY while
// stdout is piped) (#341).
func colorEnabled(w io.Writer) bool {
	return os.Getenv("NO_COLOR") == "" && terminal.IsTerminalWriter(w)
}

// Format represents the output format.
type Format string

const (
	// FormatText is the default human-readable text format.
	FormatText Format = "text"
	// FormatJSON outputs structured JSON.
	FormatJSON Format = "json"
)

// ParseFormat parses an --output value into a Format. An empty value or "text"
// is FormatText and "json" is FormatJSON; any other value is a usage error so a
// typo like "jsonn" fails loudly instead of silently printing text.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "", string(FormatText):
		return FormatText, nil
	case string(FormatJSON):
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("invalid --output value %q: must be %q or %q", s, FormatText, FormatJSON)
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
	_, _ = fmt.Fprintf(o.w, "%s %s\n", colors.FieldLabelStyle.Sprint(colorEnabled(o.w), label+":"), value)
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

// The functions below form the CLI's set of user-feedback primitives. Each
// emits a single colored line to w and serves a distinct role, so they are not
// interchangeable — pick the one whose prefix, color, and tone match the intent:
//
//   - Warning — "Warning: X" (yellow): a non-critical issue that does not stop the command.
//   - Warn    — "! X" (yellow): a per-item caveat within a larger operation.
//   - Info    — "X" (cyan): neutral status or progress, no prefix.
//   - Hint    — "Hint: X" (cyan): an actionable suggestion, usually after a Warning.
//   - Error   — "Error: X" (red): a user-facing error that is not a Go error value.
//   - Success — "✓ X": a completed action.
//   - Failed  — "Failed NAME: ERR" (red "Failed"): a per-item failure carrying its error.
//
// Warning, Warn, Info, Hint, and Error share two internal shapes (labeled and
// prefixed) so the exact byte layout of each family stays consistent.

// labeled formats a message and prints it as a single line, coloring the whole
// "label+message" string with style. A label of "" colors the message alone.
// Color tracks w's own TTY-ness so redirected stderr stays clean (#341).
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func labeled(w io.Writer, style colors.Style, label, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(w, style.Sprint(colorEnabled(w), label+msg))
}

// prefixed formats a message and prints it as "prefix message" on a single
// line, where only prefix carries color and the message stays uncolored.
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func prefixed(w io.Writer, prefix, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(w, "%s %s\n", prefix, msg)
}

// Warning prints a warning message in yellow.
// Used to alert users about non-critical issues that don't prevent command execution.
// Example: "Warning: comparing identical versions".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Warning(w io.Writer, format string, args ...any) {
	labeled(w, colors.WarningStyle, "Warning: ", format, args...)
}

// Hint prints a hint message in cyan.
// Used to provide helpful suggestions to the user, typically following a warning.
// Example: "Hint: To compare with previous version, use: suve param diff /param~1".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Hint(w io.Writer, format string, args ...any) {
	labeled(w, colors.InfoStyle, "Hint: ", format, args...)
}

// Error prints an error message in red.
// Used for user-facing error messages that are not Go errors.
// For Go errors, use the standard error return pattern instead.
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Error(w io.Writer, format string, args ...any) {
	labeled(w, colors.ErrorStyle, "Error: ", format, args...)
}

// Info prints an informational message in cyan with no prefix.
// Used for status updates, progress messages, and neutral information.
// Example: "No changes staged.", "Applied 3 staged changes".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Info(w io.Writer, format string, args ...any) {
	labeled(w, colors.InfoStyle, "", format, args...)
}

// Warn prints a warning message with a yellow "!" prefix.
// Used for per-item caveats within a larger operation.
// Example: "! Skipped /app/config (same as AWS)".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Warn(w io.Writer, format string, args ...any) {
	prefixed(w, colors.WarningStyle.Sprint(colorEnabled(w), "!"), format, args...)
}

// Success prints a success message with a green checkmark prefix.
// Used to confirm a completed action.
// Example: "✓ Created parameter /app/config".
//
//nolint:goprintffuncname // intentionally named without 'f' suffix for cleaner API
func Success(w io.Writer, format string, args ...any) {
	prefixed(w, colors.SuccessStyle.Sprint(colorEnabled(w), "✓"), format, args...)
}

// Failed prints a per-item failure message with a red "Failed" prefix,
// appending the error that caused it.
// Example: "Failed /app/config: error message".
func Failed(w io.Writer, name string, err error) {
	_, _ = fmt.Fprintf(w, "%s %s: %v\n", colors.ErrorStyle.Sprint(colorEnabled(w), "Failed"), name, err)
}

// WriteJSON encodes v as indented JSON (two-space indent) followed by a
// trailing newline, writing the result to w. It is the shared implementation
// for the CLI's --output=json modes.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
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
	colored := make([]string, len(lines))

	// The ---/+++ file labels appear only before the first @@ hunk; once inside
	// a hunk, a removed/added line whose content starts with --/++ must be
	// colored as removed/added, not misclassified as a header (#339).
	inHunk := false

	for i, line := range lines {
		switch {
		case !inHunk && (strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++")):
			colored[i] = colors.DiffHeader(line)
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			colored[i] = colors.DiffHunk(line)
		case strings.HasPrefix(line, "-"):
			colored[i] = colors.DiffRemoved(line)
		case strings.HasPrefix(line, "+"):
			colored[i] = colors.DiffAdded(line)
		default:
			colored[i] = line
		}
	}

	// Join rather than append "\n" per element: strings.Split yields a trailing
	// empty element for a "\n"-terminated diff, so appending per element added a
	// spurious extra newline vs DiffRaw (#338).
	return strings.Join(colored, "\n")
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
