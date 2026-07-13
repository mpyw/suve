package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// dialogCursor returns the real terminal cursor for the focused text field of an
// open dialog, or nil when no dialog is open or none of its fields is a text
// input. It takes the already-composited screen so the caret is read from the
// exact frame the terminal will draw.
//
// Why the app must drive the real cursor: the dialogs embed huh forms, whose
// bubbles text inputs draw a *virtual* cursor (a reverse-video cell) into their
// View string rather than exposing a caret position. That virtual cell does not
// reliably surface a visible caret — it blinks itself off, and the app renders
// under AltScreen with view.Cursor unset, which hides the real terminal cursor
// entirely — so the user cannot see where they are typing (#765). huh v2 exposes
// neither the caret position nor a real-cursor toggle, so the app instead locates
// the caret directly in the composited screen: the focused text input paints a
// steady reverse-video cell there (the app keeps it steady by swallowing the
// blink; see App.Update), and nothing else in the TUI uses reverse video, so that
// cell's screen position is the caret. Reading the final frame keeps the caret
// correct regardless of where the dialog overlay is composited.
func (m *App) dialogCursor(screen string) *tea.Cursor {
	if len(m.dialogs) == 0 {
		return nil
	}

	col, row, ok := cursorCellPos(screen)
	if !ok {
		return nil
	}

	return tea.NewCursor(col, row)
}

// cursorCellPos finds the caret by locating the first cell drawn with the
// reverse-video (SGR 7) attribute — the virtual cursor a focused bubbles text
// input paints, and the only reverse-video cell the TUI ever draws. It returns
// the caret's display column and row (col measured in terminal cells, so wide
// runes and the ANSI styling before the caret are accounted for), and ok=false
// when no such cell is present (no text input is focused).
func cursorCellPos(screen string) (col, row int, ok bool) {
	for r, line := range strings.Split(screen, "\n") {
		if idx := indexReverseSGR(line); idx >= 0 {
			return lipgloss.Width(line[:idx]), r, true
		}
	}

	return 0, 0, false
}

// indexReverseSGR returns the byte index of the first SGR escape in line that
// enables the reverse attribute, or -1. It parses each CSI ...m sequence and
// skips the multi-parameter 256-color (38;5;n) and truecolor (38;2;r;g;b) color
// specs, so a color parameter that merely contains a 7 (e.g. 38;5;7) is never
// mistaken for the reverse attribute.
func indexReverseSGR(line string) int {
	const csi = "\x1b[" // Control Sequence Introducer

	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], csi) {
			j := i + len(csi)
			for j < len(line) && (line[j] == ';' || (line[j] >= '0' && line[j] <= '9')) {
				j++
			}

			if j < len(line) && line[j] == 'm' && sgrHasReverse(line[i+len(csi):j]) {
				return i
			}

			i = j + 1

			continue
		}

		i++
	}

	return -1
}

// sgrHasReverse reports whether an SGR parameter list (the bytes between "\x1b["
// and "m") turns the reverse attribute on, correctly stepping over color
// introducers so their sub-parameters are not misread as attributes.
func sgrHasReverse(params string) bool {
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "38", "48", "58": // color introducer: skip its sub-parameters
			if i+1 < len(parts) {
				switch parts[i+1] {
				case "5": // 256-color: introducer, "5", index
					i += 2
				case "2": // truecolor: introducer, "2", r, g, b
					i += 4
				}
			}
		case "7": // reverse on
			return true
		}
	}

	return false
}
