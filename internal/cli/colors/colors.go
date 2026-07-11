// Package colors provides per-destination colored output for the CLI.
//
// Coloring is decided per destination writer rather than from a process-global:
// fatih/color's package-global NoColor is set once from whether os.Stdout is a
// TTY, which is wrong for anything written to os.Stderr (or any other writer).
// Every colored string therefore goes through a Palette bound to the writer it
// will be written to, so its color tracks that writer's own TTY-ness and stays
// correct under redirection like `suve … 2>err.log` or `suve … | cat` (#341).
package colors

import (
	"io"
	"os"

	"github.com/fatih/color"

	"github.com/mpyw/suve/internal/cli/terminal"
)

// Palette renders colored text for a single destination writer. Obtain one with
// For(w) and use its methods to color strings destined for w.
type Palette struct {
	enabled bool
}

// For returns a Palette whose coloring is enabled only when NO_COLOR is unset
// and w is a terminal. Deciding this per writer (rather than from the
// os.Stdout-derived process-global) is what keeps each stream's color correct.
func For(w io.Writer) Palette {
	return Palette{enabled: os.Getenv("NO_COLOR") == "" && terminal.IsTerminalWriter(w)}
}

// sprint colorizes the operands with attrs when the palette is enabled, and
// returns them unformatted otherwise. A fresh color.Color with a forced
// per-instance setting is used, so palettes for different destinations never
// race on (or get overridden by) the process-global color.NoColor.
func (p Palette) sprint(a []any, attrs ...color.Attribute) string {
	c := color.New(attrs...)

	if p.enabled {
		c.EnableColor()
	} else {
		c.DisableColor()
	}

	return c.Sprint(a...)
}

// Warning formats text in yellow for warning messages.
func (p Palette) Warning(a ...any) string { return p.sprint(a, color.FgYellow) }

// Error formats text in red for error messages.
func (p Palette) Error(a ...any) string { return p.sprint(a, color.FgRed) }

// Success formats text in green for success messages.
func (p Palette) Success(a ...any) string { return p.sprint(a, color.FgGreen) }

// Info formats text in cyan for informational messages.
func (p Palette) Info(a ...any) string { return p.sprint(a, color.FgCyan) }

// Failed formats "Failed" text in red.
func (p Palette) Failed(a ...any) string { return p.sprint(a, color.FgRed) }

// Version formats version numbers in yellow.
func (p Palette) Version(a ...any) string { return p.sprint(a, color.FgYellow) }

// Current formats the "(current)" marker in green.
func (p Palette) Current(a ...any) string { return p.sprint(a, color.FgGreen) }

// FieldLabel formats field labels (e.g., "Date:", "Value:") in cyan.
func (p Palette) FieldLabel(a ...any) string { return p.sprint(a, color.FgCyan) }

// DiffHeader formats diff header lines (---/+++) in cyan.
func (p Palette) DiffHeader(a ...any) string { return p.sprint(a, color.FgCyan) }

// DiffHunk formats diff hunk markers (@@) in cyan.
func (p Palette) DiffHunk(a ...any) string { return p.sprint(a, color.FgCyan) }

// DiffAdded formats added lines (+) in green.
func (p Palette) DiffAdded(a ...any) string { return p.sprint(a, color.FgGreen) }

// DiffRemoved formats removed lines (-) in red.
func (p Palette) DiffRemoved(a ...any) string { return p.sprint(a, color.FgRed) }

// OpAdd formats the add operation indicator (A) in green.
func (p Palette) OpAdd(a ...any) string { return p.sprint(a, color.FgGreen) }

// OpModify formats the modify operation indicator (M) in green.
func (p Palette) OpModify(a ...any) string { return p.sprint(a, color.FgGreen) }

// OpDelete formats the delete operation indicator (D) in red.
func (p Palette) OpDelete(a ...any) string { return p.sprint(a, color.FgRed) }
