// Package terminal provides terminal-related utilities.
package terminal

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

// DefaultWidth is the default terminal width when detection fails.
const DefaultWidth = 50

// Fder is an interface for types that have a file descriptor.
type Fder interface {
	Fd() uintptr
}

// GetSize returns the terminal width and height for the given file descriptor.
// This is a variable to allow mocking in tests.
var GetSize = term.GetSize

// IsTTY checks if the file descriptor is a TTY.
// This is a variable to allow mocking in tests.
var IsTTY = isatty.IsTerminal

// GetWidth returns the terminal width for stdout.
// Returns DefaultWidth if detection fails or stdout is not a terminal.
func GetWidth() int {
	return GetWidthFromWriter(os.Stdout)
}

// GetWidthFromWriter returns the terminal width for the given writer.
// Returns DefaultWidth if detection fails or writer is not a terminal.
func GetWidthFromWriter(w io.Writer) int {
	f, ok := w.(Fder)
	if !ok || !IsTTY(f.Fd()) {
		return DefaultWidth
	}

	width, _, err := GetSize(int(f.Fd()))
	if err != nil || width <= 0 {
		return DefaultWidth
	}
	return width
}

// IsTerminal returns true if stdout is a terminal.
func IsTerminal() bool {
	return IsTerminalWriter(os.Stdout)
}

// IsTerminalWriter returns true if the given writer is a terminal.
func IsTerminalWriter(w io.Writer) bool {
	f, ok := w.(Fder)
	if !ok {
		return false
	}
	return IsTTY(f.Fd())
}
