// Package colors provides pre-configured color functions for CLI output.
package colors

import "github.com/fatih/color"

//nolint:gochecknoglobals // Immutable color definitions initialized at package load
var (
	// Warning formats text in yellow for warning messages.
	Warning = color.New(color.FgYellow).SprintFunc()

	// Error formats text in red for error messages.
	Error = color.New(color.FgRed).SprintFunc()

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
)

// Style colorizes text for a specific destination writer, independent of the
// process-global color.NoColor (which fatih/color derives once from os.Stdout).
// Feedback primitives use it so a message's color tracks the TTY-ness of its own
// writer rather than that of os.Stdout, keeping stderr correct under redirection
// like `suve … 2>err.log` or `suve … | cat` (#341).
type Style struct {
	attrs []color.Attribute
}

// Sprint colorizes the operands with the style's attributes when enabled, and
// returns them unformatted otherwise. A fresh color.Color with a forced
// per-instance setting is used, so writers to different destinations never race
// on (or get overridden by) the process-global color.NoColor.
func (s Style) Sprint(enabled bool, a ...any) string {
	c := color.New(s.attrs...)

	if enabled {
		c.EnableColor()
	} else {
		c.DisableColor()
	}

	return c.Sprint(a...)
}

//nolint:gochecknoglobals // Immutable style definitions initialized at package load
var (
	// WarningStyle is yellow, for warning text.
	WarningStyle = Style{attrs: []color.Attribute{color.FgYellow}}

	// ErrorStyle is red, for error text.
	ErrorStyle = Style{attrs: []color.Attribute{color.FgRed}}

	// SuccessStyle is green, for success markers.
	SuccessStyle = Style{attrs: []color.Attribute{color.FgGreen}}

	// InfoStyle is cyan, for informational/hint text.
	InfoStyle = Style{attrs: []color.Attribute{color.FgCyan}}

	// FieldLabelStyle is cyan, for field labels.
	FieldLabelStyle = Style{attrs: []color.Attribute{color.FgCyan}}
)
