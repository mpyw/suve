package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/styles"
)

// ListRow is one presentation-ready entry row. The owning page precomputes the
// value preview (masked when appropriate) and badges, so the list widget stays
// dumb — it never sees a raw secret value.
type ListRow struct {
	// Name is the entry name (the primary column).
	Name string
	// Preview is an optional value preview shown after the name in values-mode
	// (already masked by the caller for secrets); empty hides it.
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
// selection visible.
func (l *EntryList) Move(delta int) {
	if len(l.rows) == 0 {
		return
	}

	l.selected = clamp(l.selected+delta, 0, len(l.rows)-1)
	l.ensureVisible()
}

// SelectIndex selects a specific index (clamped) and keeps it visible.
func (l *EntryList) SelectIndex(i int) {
	if len(l.rows) == 0 {
		return
	}

	l.selected = clamp(i, 0, len(l.rows)-1)
	l.ensureVisible()
}

// Scroll moves the viewport by delta rows without moving the selection (wheel
// scrolling), clamped so it never scrolls past the ends.
func (l *EntryList) Scroll(delta int) {
	l.offset = clamp(l.offset+delta, 0, l.maxOffset())
}

// RowAtLine maps a 0-based content line (relative to the list body, i.e. below
// its title/border) to a row index, or (0, false) when the line is the footer
// or past the last visible row. Clicks derive their target through this, never
// a hard-coded coordinate.
func (l *EntryList) RowAtLine(line int) (int, bool) {
	if line < 0 || len(l.rows) == 0 {
		return 0, false
	}

	idx := l.offset + line
	if idx >= len(l.rows) || line >= l.visibleRows() {
		return 0, false
	}

	return idx, true
}

// View renders the list body (without a title/border; the page frames it) into
// width×height. The selected row carries a cursor; a load-more footer takes the
// last line when rows are truncated.
func (l *EntryList) View() string {
	if l.height <= 0 || l.width <= 0 {
		return ""
	}

	visible := l.visibleRows()
	lines := make([]string, 0, l.height)

	for i := range visible {
		idx := l.offset + i
		if idx >= len(l.rows) {
			break
		}

		lines = append(lines, l.renderRow(idx))
	}

	if l.hasMore {
		lines = append(lines, l.styles.PageHint.Render(truncate("  … load more (L)", l.width)))
	}

	// Pad to the full height so the pane border stays rectangular.
	for len(lines) < l.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// renderRow renders one row: a cursor for the selection, the name, an optional
// value preview, and trailing badges, all clamped to width.
func (l *EntryList) renderRow(idx int) string {
	row := l.rows[idx]

	cursor := "  "
	if idx == l.selected {
		cursor = l.styles.StatusValue.Render("▸ ")
	}

	name := row.Name
	if idx == l.selected {
		name = l.styles.StatusValue.Render(name)
	}

	left := cursor + name
	if row.Preview != "" {
		left += l.styles.PageHint.Render("  " + row.Preview)
	}

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

// visibleRows is how many entry rows fit, reserving one line for the load-more
// footer when there is a next page.
func (l *EntryList) visibleRows() int {
	if l.hasMore {
		return max(l.height-1, 0)
	}

	return l.height
}

// maxOffset is the furthest the list can scroll so the last row stays visible.
func (l *EntryList) maxOffset() int {
	return max(len(l.rows)-l.visibleRows(), 0)
}

// ensureVisible scrolls the viewport so the selection is on-screen.
func (l *EntryList) ensureVisible() {
	visible := l.visibleRows()
	if visible <= 0 {
		l.offset = 0

		return
	}

	if l.selected < l.offset {
		l.offset = l.selected
	}

	if l.selected >= l.offset+visible {
		l.offset = l.selected - visible + 1
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
