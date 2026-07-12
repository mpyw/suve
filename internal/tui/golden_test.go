//nolint:testpackage // white-box: shares the tui package's golden/teatest harness
package tui

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/assert"
)

// Fixed virtual-terminal size the shell goldens render at. It matches the
// teatest term size (WithInitialTermSize) so the frame's absolute cursor
// positioning lands on the same cells in the emulator.
const (
	goldenTermWidth  = 100
	goldenTermHeight = 30
)

// renderVisibleScreen replays a captured teatest byte stream through a fixed
// virtual terminal and returns the VISIBLE SCREEN: rows joined by newline,
// trailing spaces and trailing blank rows trimmed.
//
// Why render instead of goldening the raw bytes: teatest captures the terminal's
// whole capability handshake — alt-screen enter/exit, DECRQM synchronized-output
// (ESC[?2026$p / ESC[?2027$p) and Kitty keyboard (ESC[?u) queries, mouse and
// bracketed-paste mode sets — and CI negotiates a different handshake than a
// local run even when the drawn frame is byte-identical. A cell-grid emulator
// applies the drawing commands and ignores the invisible probes, so the golden
// captures exactly what a user sees and is environment-independent. This harness
// is shared by the later TUI steps, so it is written to be reused.
func renderVisibleScreen(tb testing.TB, raw []byte) string {
	tb.Helper()

	e := vt.NewEmulator(goldenTermWidth, goldenTermHeight)

	// Drain the emulator's reply pipe. Capability queries make the emulator write
	// a response; with no reader that write blocks the emulator forever. Draining
	// answers and discards them — the responses touch no cell, which is exactly
	// why the CI-only queries never perturb the rendered screen.
	done := make(chan struct{})
	go func() {
		defer close(done)

		_, _ = io.Copy(io.Discard, e)
	}()

	// LNM (ESC[20h): a bare line feed also returns to column 0. Bubble Tea stacks
	// the top chrome rows (status / tab bar / separator) with bare LFs, so without
	// LNM the emulator would staircase them.
	_, _ = e.WriteString("\x1b[20h")

	// Drop the alt-screen exit (ESC[?1049l). The captured stream ends by leaving
	// the alt screen on quit, which returns to the blank primary buffer; without
	// this the emulator's visible screen would be empty. The rest of the handshake
	// is harmless to replay onto the cell grid.
	_, _ = e.Write(bytes.ReplaceAll(raw, []byte("\x1b[?1049l"), nil))

	_ = e.Close()

	<-done

	// Emulator.String() already trims trailing spaces per line; also drop trailing
	// blank rows so the golden ends at the last drawn line.
	return strings.TrimRight(e.String(), "\n")
}

// TestRenderVisibleScreen_AbsorbsCapabilityPreamble is the CI-divergence
// regression proof. It feeds the two capability handshakes actually observed —
// a local run, and the CI run which additionally emits DECRQM synchronized-
// output queries (ESC[?2026$p, ESC[?2027$p) and a Kitty keyboard query (ESC[?u)
// — followed by an identical drawn frame, and asserts the rendered visible
// screen is byte-identical. The local machine only ever produces the local-style
// stream, so this test is the sole exercise of the CI-style stream: it proves
// renderVisibleScreen absorbs the divergence the raw-byte goldens tripped on.
func TestRenderVisibleScreen_AbsorbsCapabilityPreamble(t *testing.T) {
	t.Parallel()

	// A short frame body: home + clear, a status row, a bare-LF second row, then
	// an absolutely-positioned cell. Enough to exercise LF handling and cursor
	// positioning without depending on the real shell layout.
	const body = "\x1b[2Jsuve  aws\n Param Secret\x1b[16;36H(hello)"

	// The two preambles differ ONLY in the invisible capability probes: CI prepends
	// the two DECRQM queries and inserts a Kitty keyboard query before ESC[H.
	const localPreamble = "\x1b[>4m\x1b[=0;1u\x1b[?1049h\x1b[?25l\x1b[?2004h" +
		"\x1b[?1002h\x1b[?1006h\x1b[>4;2m\x1b[=1;1u\x1b[H"

	const ciPreamble = "\x1b[?2026$p\x1b[?2027$p\x1b[>4m\x1b[=0;1u\x1b[?1049h\x1b[?25l\x1b[?2004h" +
		"\x1b[?1002h\x1b[?1006h\x1b[>4;2m\x1b[=1;1u\x1b[?u\x1b[H"

	local := renderVisibleScreen(t, []byte(localPreamble+body))
	ci := renderVisibleScreen(t, []byte(ciPreamble+body))

	assert.Equal(t, local, ci, "the CI capability handshake must render the same visible screen as the local one")
	assert.Contains(t, local, "suve  aws", "sanity: the frame content actually rendered")
	assert.Contains(t, local, "(hello)", "sanity: absolutely-positioned content rendered")
}
