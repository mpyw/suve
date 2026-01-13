// Package colors provides pre-configured color functions for CLI output.
package colors

import "github.com/fatih/color"

//nolint:gochecknoglobals // Immutable color definitions initialized at package load
var (
	// Warning formats text in yellow for warning messages.
	Warning = color.New(color.FgYellow).SprintFunc()

	// Error formats text in red for error messages.
	Error = color.New(color.FgRed).SprintFunc()

	// Success formats text in green for success messages.
	Success = color.New(color.FgGreen).SprintFunc()

	// Info formats text in cyan for informational messages.
	Info = color.New(color.FgCyan).SprintFunc()

	// Version formats version numbers in yellow.
	Version = color.New(color.FgYellow).SprintFunc()

	// Current formats "(current)" marker in green.
	Current = color.New(color.FgGreen).SprintFunc()

	// FieldLabel formats field labels (e.g., "Date:", "Value:") in cyan.
	FieldLabel = color.New(color.FgCyan).SprintFunc()

	// DiffHeader formats diff header lines (---/+++) in cyan.
	DiffHeader = color.New(color.FgCyan).SprintFunc()

	// DiffHunk formats diff hunk markers (@@) in cyan.
	DiffHunk = color.New(color.FgCyan).SprintFunc()

	// DiffAdded formats added lines (+) in green.
	DiffAdded = color.New(color.FgGreen).SprintFunc()

	// DiffRemoved formats removed lines (-) in red.
	DiffRemoved = color.New(color.FgRed).SprintFunc()

	// OpAdd formats add operation indicator (A) in green.
	OpAdd = color.New(color.FgGreen).SprintFunc()

	// OpModify formats modify operation indicator (M) in green.
	OpModify = color.New(color.FgGreen).SprintFunc()

	// OpDelete formats delete operation indicator (D) in red.
	OpDelete = color.New(color.FgRed).SprintFunc()

	// Failed formats "Failed" text in red.
	Failed = color.New(color.FgRed).SprintFunc()
)
