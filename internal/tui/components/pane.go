package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/styles"
)

// Pane frames a titled body inside a rounded border sized to width×height (the
// OUTER size). The title occupies the first inner line; the body fills the rest.
// Every content line is normalized to exactly the inner width (truncated or
// padded) so the border stays rectangular and no line ever re-wraps — the bug
// that split a right-aligned badge onto its own row. Centralizing the framing
// keeps every pane identical, which also keeps goldens stable.
func Pane(st styles.Styles, title, body string, width, height int) string {
	return renderPane(st.Pane, st, title, body, width, height)
}

// PaneFocused frames a pane exactly like Pane but with the focused border, so the
// pane that currently holds keyboard focus is visually distinct from the idle one.
func PaneFocused(st styles.Styles, title, body string, width, height int) string {
	return renderPane(st.PaneFocused, st, title, body, width, height)
}

// renderPane is the shared framing body: frame is the border style to draw with
// (Pane or PaneFocused), so the inner content sizing/normalization stays identical
// and goldens stay stable regardless of the focus border.
func renderPane(frame lipgloss.Style, st styles.Styles, title, body string, width, height int) string {
	innerW, innerH := PaneInner(width, height)
	if innerW <= 0 || innerH < 0 {
		return ""
	}

	rows := make([]string, 0, innerH+1)
	rows = append(rows, normalizeLine(st.PaneTitle.Render(title), innerW))

	for _, line := range splitLimit(body, innerH) {
		rows = append(rows, normalizeLine(line, innerW))
	}

	for len(rows) < innerH+1 {
		rows = append(rows, strings.Repeat(" ", innerW))
	}

	return frame.Render(strings.Join(rows, "\n"))
}

// Pane chrome sizes: two border columns, and two border rows plus one title row.
const (
	paneBorderCols = 2
	paneChromeRows = 3
)

// PaneInner returns the inner content size available inside a Pane of the given
// outer size: width minus the two border columns, and height minus the two
// border rows and the one title row.
func PaneInner(width, height int) (int, int) {
	return max(width-paneBorderCols, 0), max(height-paneChromeRows, 0)
}

// splitLimit splits body into at most limit lines.
func splitLimit(body string, limit int) []string {
	if body == "" {
		return nil
	}

	lines := strings.Split(body, "\n")
	if len(lines) > limit {
		return lines[:limit]
	}

	return lines
}

// normalizeLine truncates or space-pads a (possibly styled) line to exactly
// width display columns, so a row of content never re-wraps inside the border.
func normalizeLine(line string, width int) string {
	w := lipgloss.Width(line)
	if w == width {
		return line
	}

	if w > width {
		return lipgloss.NewStyle().MaxWidth(width).Render(line)
	}

	return line + strings.Repeat(" ", width-w)
}
