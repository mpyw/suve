//go:build e2e

package e2e_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
)

// renderE2EWidth/renderE2EHeight size the emulator the TUI e2e helpers render
// into. They are larger than the tests' real terminal (120×34) so replaying a
// stream drawn for that size never re-wraps a marker across lines: content is
// placed at the columns it was drawn to and the extra blank columns/rows are
// trimmed.
const (
	renderE2EWidth  = 240
	renderE2EHeight = 80
)

// renderTUIScreen replays a captured teatest byte stream (or a settled model's
// View().Content) through a cell-grid virtual terminal and returns the VISIBLE
// SCREEN: rows joined by newline, trailing spaces and trailing blank rows trimmed.
//
// This mirrors the internal/tui golden harness (renderVisibleScreenSize) and
// exists for the same reason: teatest captures the terminal's whole capability
// handshake and CI emits incremental cell-updates, so a marker can be split across
// cursor-positioned writes and the drawing bytes differ from a local run even when
// the drawn frame is identical. A cell-grid emulator applies the drawing commands
// and ignores the invisible probes, so matching against its screen captures exactly
// what a user sees and is environment-independent.
func renderTUIScreen(tb testing.TB, raw []byte) string {
	tb.Helper()

	e := vt.NewEmulator(renderE2EWidth, renderE2EHeight)

	// Drain the emulator's reply pipe: capability queries make it write a response
	// that would otherwise block forever with no reader. The responses touch no
	// cell, so discarding them never perturbs the rendered screen.
	done := make(chan struct{})
	go func() {
		defer close(done)

		_, _ = io.Copy(io.Discard, e)
	}()

	// LNM (ESC[20h): a bare line feed also returns to column 0, matching how Bubble
	// Tea stacks the top chrome rows with bare LFs.
	_, _ = e.WriteString("\x1b[20h")

	// Drop the alt-screen exit (ESC[?1049l): the captured stream ends by leaving the
	// alt screen on quit, which would return to the blank primary buffer; without
	// this the visible screen would be empty. (A settled View().Content contains no
	// such sequence, so this is a harmless no-op there.)
	_, _ = e.Write(bytes.ReplaceAll(raw, []byte("\x1b[?1049l"), nil))

	screen := strings.TrimRight(e.String(), "\n")

	// Shut down without racing the drain goroutine: closing the reply pipe makes the
	// in-flight Read return EOF, so the goroutine has exited by the time <-done
	// returns; only then is it safe to Close the emulator with no reader in flight.
	if c, ok := e.InputPipe().(io.Closer); ok {
		_ = c.Close()
	}

	<-done

	_ = e.Close()

	return screen
}
