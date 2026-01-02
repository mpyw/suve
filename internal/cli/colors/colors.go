// Package colors provides pre-configured color functions for CLI output.
package colors

import "github.com/fatih/color"

var (
	// Warning formats text in yellow for warning messages.
	Warning = color.New(color.FgYellow).SprintFunc()

	// Error formats text in red for error messages.
	Error = color.New(color.FgRed).SprintFunc()

	// Success formats text in green for success messages.
	Success = color.New(color.FgGreen).SprintFunc()

	// Info formats text in cyan for informational messages.
	Info = color.New(color.FgCyan).SprintFunc()
)
