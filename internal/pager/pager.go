// Package pager provides terminal pager functionality for long outputs.
package pager

import (
	"bytes"
	"io"

	"github.com/mattn/go-isatty"
	"github.com/walles/moor/v2/pkg/moor"
)

// fder is an interface for types that have a file descriptor.
type fder interface {
	Fd() uintptr
}

// WithPagerWriter executes fn with pager support.
// If noPager is true or stdout is not a TTY, output goes directly to the provided writer.
// Otherwise, output is collected and displayed through moor pager.
func WithPagerWriter(stdout io.Writer, noPager bool, fn func(w io.Writer) error) error {
	if noPager {
		return fn(stdout)
	}

	// Check if stdout supports Fd() for TTY detection
	if f, ok := stdout.(fder); ok && isatty.IsTerminal(f.Fd()) {
		// Real TTY - use pager
		var buf bytes.Buffer
		if err := fn(&buf); err != nil {
			return err
		}

		if buf.Len() == 0 {
			return nil
		}

		return moor.PageFromString(buf.String(), moor.Options{})
	}

	// Not a TTY or doesn't support Fd() - write directly
	return fn(stdout)
}
