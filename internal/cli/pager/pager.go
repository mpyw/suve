// Package pager provides terminal pager functionality for long outputs.
package pager

import (
	"bytes"
	"io"
	"strings"

	"github.com/walles/moor/v2/pkg/moor"

	"github.com/mpyw/suve/internal/cli/terminal"
)

// WithPagerWriter executes fn with pager support.
// If noPager is true or stdout is not a TTY, output goes directly to the provided writer.
// If the output fits within the terminal height, it's written directly without paging.
// Otherwise, output is displayed through moor pager.
func WithPagerWriter(stdout io.Writer, noPager bool, fn func(w io.Writer) error) error {
	if noPager {
		return fn(stdout)
	}

	// Check if stdout supports Fd() for TTY detection
	f, ok := stdout.(terminal.Fder)
	if !ok || !terminal.IsTTY(f.Fd()) {
		// Not a TTY or doesn't support Fd() - write directly
		return fn(stdout)
	}

	// Real TTY - collect output first. The buffer still reports the real
	// terminal's Fd (see fdBuffer) so width/TTY detection during rendering
	// (e.g. log --oneline's auto width) sees the actual terminal instead of the
	// width-less buffer, which would otherwise fall back to DefaultWidth.
	buf := &fdBuffer{Buffer: &bytes.Buffer{}, fd: f.Fd()}
	if err := fn(buf); err != nil {
		return err
	}

	if buf.Len() == 0 {
		return nil
	}

	// Check if output fits in terminal
	if fitsInTerminal(terminal.FdToInt(f.Fd()), buf.String()) {
		_, err := stdout.Write(buf.Bytes())

		return err
	}

	return moor.PageFromString(buf.String(), moor.Options{})
}

// fdBuffer buffers output for paging while still reporting the underlying
// terminal's file descriptor via Fd(). This lets terminal width/TTY detection
// see the real terminal even though output is buffered before paging. Color
// output is gated globally (fatih/color), not by this writer's Fd, so exposing
// it here does not change colorization.
type fdBuffer struct {
	*bytes.Buffer

	fd uintptr
}

// Fd returns the underlying terminal's file descriptor.
func (b *fdBuffer) Fd() uintptr { return b.fd }

// fitsInTerminal returns true if the content fits within the terminal height.
// Returns false if terminal size cannot be determined.
func fitsInTerminal(fd int, content string) bool {
	_, height, err := terminal.GetSize(fd)
	if err != nil || height <= 0 {
		return false
	}

	// Count lines in content (including wrapped lines would be ideal,
	// but for simplicity we just count newlines)
	lines := strings.Count(content, "\n")
	// Add 1 if content doesn't end with newline
	if len(content) > 0 && content[len(content)-1] != '\n' {
		lines++
	}

	// Leave some margin (1 line for prompt)
	return lines < height
}
