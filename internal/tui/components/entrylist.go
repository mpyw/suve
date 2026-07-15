package components

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/styles"
)

// ListRow is one presentation-ready entry row. The owning page precomputes the
// value preview and badges, so the list widget stays dumb.
type ListRow struct {
	// Name is the entry name (the primary column).
	Name string
	// Preview is an optional value preview rendered on an indented SECOND line
	// beneath the name (values-mode); empty draws no value line, so the row is a
	// single line. The caller flattens/truncates it — an explicit values:on is a
	// reveal, so a secret value is shown, mirroring the GUI (#734).
	Preview string
	// Badges are trailing chips (e.g. "staged", "(NULL)") shown right of the row.
	Badges []string
}

// EntryList is a scrollable, single-select list of entry rows. It renders its
// own selection cursor and hit-tests row clicks against the same layout it
// draws (so a click reduces to the same selection a key move produces). It holds
// no Bubble Tea machinery: the page drives it with explicit Move/Select calls.
type EntryList struct {
	rows     []ListRow
	selected int
	offset   int
	width    int
	height   int
	styles   styles.Styles
	// focused reports whether the list pane currently holds keyboard focus. The
	// selected row is drawn with the active cursor when focused and a dimmed cursor
	// when not, so the list and history never look equally selected at once.
	focused bool
	// hasMore, when true, appends a "… load more (L)" footer occupying the last
	// visible row. It is set only when the source reports a real next page (a
	// non-empty NextToken); the un-loaded count is unknown, so no phantom number
	// is shown.
	hasMore bool
}

// NewEntryList builds an empty list with the given styles.
func NewEntryList(st styles.Styles) EntryList {
	return EntryList{styles: st}
}

// SetRows replaces the rows, clamping the selection into range. hasMore appends
// the load-more footer; pass true only when the source reports a real next page.
func (l *EntryList) SetRows(rows []ListRow, hasMore bool) {
	l.rows = rows
	l.hasMore = hasMore
	l.clampSelection()
	l.ensureVisible()
}

// SetFocused sets whether the list pane holds keyboard focus, which selects the
// active vs dimmed selection style the next View draws with.
func (l *EntryList) SetFocused(focused bool) { l.focused = focused }

// SetSize sets the list's inner content size (inside its border).
func (l *EntryList) SetSize(width, height int) {
	l.width = max(width, 0)
	l.height = max(height, 0)
	l.ensureVisible()
}

// Len returns the number of rows.
func (l *EntryList) Len() int { return len(l.rows) }

// Selected returns the selected index (or -1 when empty).
func (l *EntryList) Selected() int {
	if len(l.rows) == 0 {
		return -1
	}

	return l.selected
}

// SelectedRow returns the selected row and true, or a zero row and false when
// the list is empty.
func (l *EntryList) SelectedRow() (ListRow, bool) {
	if len(l.rows) == 0 {
		return ListRow{}, false
	}

	return l.rows[l.selected], true
}

// Move changes the selection by delta, clamping at the ends, and keeps the
// selection visible. It reports whether the viewport actually scrolled (the
// offset changed), so callers can force a full repaint only when a scroll-region
// optimization would otherwise fire (see internal/tui/termquirk).
func (l *EntryList) Move(delta int) bool {
	if len(l.rows) == 0 {
		return false
	}

	before := l.offset
	l.selected = clamp(l.selected+delta, 0, len(l.rows)-1)
	l.ensureVisible()

	return l.offset != before
}

// SelectIndex selects a specific index (clamped) and keeps it visible. It reports
// whether the offset changed (a click on a partially-visible row scrolls it fully
// into view), so callers can force a full repaint only then (see termquirk).
func (l *EntryList) SelectIndex(i int) bool {
	if len(l.rows) == 0 {
		return false
	}

	before := l.offset
	l.selected = clamp(i, 0, len(l.rows)-1)
	l.ensureVisible()

	return l.offset != before
}

// Scroll moves the viewport by delta rows without moving the selection (wheel
// scrolling), clamped so it never scrolls past the ends. It reports whether the
// offset actually changed (false when already clamped at an end), so callers can
// force a full repaint only on a real scroll (see internal/tui/termquirk).
func (l *EntryList) Scroll(delta int) bool {
	before := l.offset
	l.offset = clamp(l.offset+delta, 0, l.maxOffset())

	return l.offset != before
}

// RowAtLine maps a 0-based content line (relative to the list body, i.e. below
// its title/border) to a row index, or (0, false) when the line is the footer
// or past the last visible row. A row's name line and its value line both map
// back to that row, so a click anywhere in a row selects it. Clicks derive their
// target through this, never a hard-coded coordinate.
func (l *EntryList) RowAtLine(line int) (int, bool) {
	if line < 0 || len(l.rows) == 0 {
		return 0, false
	}

	_, rowOf := l.window()
	if line >= len(rowOf) {
		return 0, false
	}

	return rowOf[line], true
}

// View renders the list body (without a title/border; the page frames it) into
// width×height. The selected row carries a cursor; a load-more footer takes the
// last line when rows are truncated.
func (l *EntryList) View() string {
	if l.height <= 0 || l.width <= 0 {
		return ""
	}

	lines, _ := l.window()

	if l.hasMore {
		lines = append(lines, l.styles.PageHint.Render(truncate("  … load more (L)", l.width)))
	}

	// Pad to the full height so the pane border stays rectangular.
	for len(lines) < l.height {
		lines = append(lines, "")
	}

	return strings.Join(lines[:l.height], "\n")
}

// window renders the visible content lines for the current offset (capped at the
// visible line budget) and, parallel to them, the row index each line belongs
// to — so View draws them and RowAtLine maps a clicked line back to its row
// without the two drifting apart (mirrors HistoryTable.window).
func (l *EntryList) window() (lines []string, rowOf []int) {
	visible := l.visibleRows()

	for row := l.offset; row < len(l.rows) && len(lines) < visible; row++ {
		for _, line := range l.rowLines(row) {
			if len(lines) >= visible {
				break
			}

			lines = append(lines, line)
			rowOf = append(rowOf, row)
		}
	}

	return lines, rowOf
}

// rowLines returns one row's rendered lines: the name header line (cursor + name
// + right-aligned badges) and, when a value preview is present, an indented value
// line beneath it (values-mode). The value line shares the history table's indent
// so it reads as a detail of the name above it (#734).
func (l *EntryList) rowLines(idx int) []string {
	lines := []string{l.renderRow(idx)}

	if preview := l.rows[idx].Preview; preview != "" {
		lines = append(lines, l.styles.PageHint.Render(truncate("     "+preview, l.width)))
	}

	return lines
}

// renderRow renders one row's header line: a cursor for the selection, the name,
// and trailing right-aligned badges, all clamped to width. The value preview is
// drawn separately on the next line by rowLines.
func (l *EntryList) renderRow(idx int) string {
	row := l.rows[idx]

	cursor := "  "
	name := row.Name

	if idx == l.selected {
		// A filled cursor + active style when focused; a hollow cursor + dimmed
		// style when not, so the selection stays visible but clearly idle.
		if l.focused {
			cursor = l.styles.Selection.Render("▸ ")
			name = l.styles.Selection.Render(name)
		} else {
			cursor = l.styles.SelectionInactive.Render("▹ ")
			name = l.styles.SelectionInactive.Render(name)
		}
	}

	left := cursor + name

	badges := renderBadges(l.styles, row.Badges)
	if badges == "" {
		return truncate(left, l.width)
	}

	// Right-align the badges within the row width when there is room.
	gap := l.width - lipgloss.Width(left) - lipgloss.Width(badges)
	if gap < 1 {
		return truncate(left+" "+badges, l.width)
	}

	return truncate(left+strings.Repeat(" ", gap)+badges, l.width)
}

// visibleRows is the window's height in rendered lines, reserving one line for
// the load-more footer when there is a next page. It is the per-View line budget
// window() fills, not a row count: a values-mode row renders as two lines, so the
// offset math below converts between the two by summing per-row line counts.
func (l *EntryList) visibleRows() int {
	if l.hasMore {
		return max(l.height-1, 0)
	}

	return l.height
}

// maxOffset caps scrolling so the last row's final line stays reachable at the
// bottom of the window. Because a values-mode row renders as two lines, this is
// not len(rows)-visible: it is the smallest row offset whose rows [offset..end]
// fill (without exceeding) the line budget, found by walking rows from the end
// and accumulating their rendered line counts until the next row would overflow.
func (l *EntryList) maxOffset() int {
	budget := l.visibleRows()
	if budget <= 0 || len(l.rows) == 0 {
		return 0
	}

	total := 0
	offset := len(l.rows)

	for i := range slices.Backward(l.rows) {
		h := len(l.rowLines(i))
		if total+h > budget {
			break
		}

		total += h
		offset = i
	}

	// When even the last row alone overflows the budget, it can still be scrolled
	// to (its top line at the pane top), so cap the offset at the last row.
	return min(offset, len(l.rows)-1)
}

// ensureVisible keeps the selected row's rendered lines fully inside the
// line-budget window. It scrolls up when the selection starts above the window,
// and down when the selection's lines extend past the window bottom — walking up
// from the selected row accumulating line counts to find the smallest offset that
// still shows the selected row's final line — then clamps to the valid range.
// Line counts come from rowLines, the same source window() uses, so the scroll
// math and the mouse hit-map never disagree.
func (l *EntryList) ensureVisible() {
	budget := l.visibleRows()
	if budget <= 0 || len(l.rows) == 0 {
		l.offset = 0

		return
	}

	if l.selected < l.offset {
		l.offset = l.selected
	}

	// Find the smallest offset whose rows [offset..selected] fit the budget, so
	// the selected row's last line lands at or above the window bottom.
	total := 0
	minOffset := l.selected

	for i := l.selected; i >= 0; i-- {
		h := len(l.rowLines(i))
		if total+h > budget {
			break
		}

		total += h
		minOffset = i
	}

	if l.offset < minOffset {
		l.offset = minOffset
	}

	l.offset = clamp(l.offset, 0, l.maxOffset())
}

// clampSelection keeps the selection within the current rows.
func (l *EntryList) clampSelection() {
	if len(l.rows) == 0 {
		l.selected = 0

		return
	}

	l.selected = clamp(l.selected, 0, len(l.rows)-1)
}

// renderBadges renders trailing chips as "[a] [b]".
func renderBadges(st styles.Styles, badges []string) string {
	if len(badges) == 0 {
		return ""
	}

	parts := make([]string, len(badges))
	for i, b := range badges {
		parts[i] = st.PageHint.Render("[" + b + "]")
	}

	return strings.Join(parts, " ")
}

// clamp constrains v to [lo, hi]. lo is a parameter (rather than a hardcoded 0)
// so the shared list/history widgets read as a general clamp; today every caller
// passes 0, which is fine.
//
//nolint:unparam // general-purpose clamp shared by the list and history widgets
func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}

	return max(lo, min(v, hi))
}
