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

	// Interior padding (#698) is applied here, in the single framing path, so it
	// wraps the already-normalized inner block and every pane (list, detail, diff)
	// stays identical. PaneInner subtracts the same padding, so the padded block
	// plus border still totals exactly width×height and no line re-wraps.
	return frame.Padding(panePadY, panePadX).Render(strings.Join(rows, "\n"))
}

// Pane chrome sizes: the border, the title row, and the interior padding added
// inside the border so content is not jammed against the box edge (#698).
// Vertical padding is 0 to preserve the row budget on short terminals; horizontal
// padding is one column a side.
const (
	paneBorderCols = 2 // left + right border columns
	paneBorderRows = 2 // top + bottom border rows
	paneTitleRows  = 1 // the title occupies the first inner line
	panePadX       = 1 // interior horizontal padding, each side
	panePadY       = 0 // interior vertical padding, each side

	paneChromeCols = paneBorderCols + 2*panePadX
	paneChromeRows = paneBorderRows + paneTitleRows + 2*panePadY
)

// PaneInner returns the inner content size available inside a Pane of the given
// outer size: width minus the border columns and the horizontal interior
// padding, and height minus the border rows, the title row, and the vertical
// interior padding.
func PaneInner(width, height int) (int, int) {
	return max(width-paneChromeCols, 0), max(height-paneChromeRows, 0)
}

// PaneContentTop is the row a pane's first content line (its title) sits on,
// measured from the pane's top edge: the top border plus the top interior
// padding, then the title row for the body beneath it. Pages that hit-test pane
// content derive their region origins from this (and PaneContentLeft) so the
// mouse map can never drift from renderPane's actual geometry (#661/#663/#698).
func PaneContentTop() int { return paneBorderRows/2 + panePadY + paneTitleRows }

// PaneContentLeft is the column a pane's content starts on, measured from the
// pane's left edge: the left border plus the left interior padding. See
// PaneContentTop for why hit-testing pages derive origins from these helpers.
func PaneContentLeft() int { return paneBorderCols/2 + panePadX }

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
