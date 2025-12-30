// Package output handles formatted output for the CLI.
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

// ValuePreview prints a truncated preview of a value.
func (o *Writer) ValuePreview(value string, maxLen int) {
	if len(value) > maxLen {
		value = value[:maxLen] + "..."
	}
	// Replace newlines with spaces for preview
	value = strings.ReplaceAll(value, "\n", " ")
	gray := color.New(color.FgHiBlack).SprintFunc()
	_, _ = fmt.Fprintf(o.w, "%s\n", gray(value))
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
