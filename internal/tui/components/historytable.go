package components

import (
	"strings"

	"github.com/mpyw/suve/internal/tui/styles"
)

// comparePickCount is the number of history rows a diff compares (exactly two).
const comparePickCount = 2

// HistoryEntry is one presentation-ready version row. Badges (state or staging
// labels) and the per-version tag line are precomputed by the page so the
// widget never interprets provider metadata.
type HistoryEntry struct {
	// Label is the version label ("#14" or a shortened id).
	Label string
	// Date is the pre-formatted date, empty when unknown.
	Date string
	// Current marks the current version.
	Current bool
	// Badges are trailing chips (state or staging labels).
	Badges []string
	// TagsLine is an indented per-version tag line (Azure Key Vault), empty when
	// the provider keeps tags at the resource level.
	TagsLine string
}

// HistoryTable is a scrollable, single-select version table with an optional
// compare mode in which up to two rows are marked for a diff. It renders its own
// cursor and compare markers and hit-tests row clicks against the same layout it
// draws.
type HistoryTable struct {
	rows     []HistoryEntry
	selected int
	offset   int
	width    int
	height   int
	styles   styles.Styles
	compare  bool
	// picks holds the row indices selected for comparison (0, 1, or 2 entries).
	picks []int
}

// NewHistoryTable builds an empty history table.
func NewHistoryTable(st styles.Styles) HistoryTable {
	return HistoryTable{styles: st}
}

// SetRows replaces the rows and resets scrolling/compare picks.
func (t *HistoryTable) SetRows(rows []HistoryEntry) {
	t.rows = rows
	t.selected = 0
	t.offset = 0
	t.picks = nil
	t.ensureVisible()
}

// SetSize sets the table's inner content size.
func (t *HistoryTable) SetSize(width, height int) {
	t.width = max(width, 0)
	t.height = max(height, 0)
	t.ensureVisible()
}

// Len returns the row count.
func (t *HistoryTable) Len() int { return len(t.rows) }

// Selected returns the selected index, or -1 when empty.
func (t *HistoryTable) Selected() int {
	if len(t.rows) == 0 {
		return -1
	}

	return t.selected
}

// Move changes the selection by delta, clamped, keeping it visible.
func (t *HistoryTable) Move(delta int) {
	if len(t.rows) == 0 {
		return
	}

	t.selected = clamp(t.selected+delta, 0, len(t.rows)-1)
	t.ensureVisible()
}

// SelectIndex selects a specific row index (clamped).
func (t *HistoryTable) SelectIndex(i int) {
	if len(t.rows) == 0 {
		return
	}

	t.selected = clamp(i, 0, len(t.rows)-1)
	t.ensureVisible()
}

// Scroll wheel-scrolls without moving the selection.
func (t *HistoryTable) Scroll(delta int) {
	t.offset = clamp(t.offset+delta, 0, t.maxOffset())
}

// SetCompare toggles compare mode; leaving it clears the picks.
func (t *HistoryTable) SetCompare(on bool) {
	t.compare = on
	if !on {
		t.picks = nil
	}
}

// Compare reports whether compare mode is on.
func (t *HistoryTable) Compare() bool { return t.compare }

// TogglePick marks/unmarks the selected row for comparison. A third pick evicts
// the oldest so at most two rows are ever selected.
func (t *HistoryTable) TogglePick() {
	if len(t.rows) == 0 {
		return
	}

	i := t.selected
	if pos := indexOf(t.picks, i); pos >= 0 {
		t.picks = append(t.picks[:pos], t.picks[pos+1:]...)

		return
	}

	t.picks = append(t.picks, i)
	if len(t.picks) > comparePickCount {
		t.picks = t.picks[len(t.picks)-comparePickCount:]
	}
}

// PickedVersions returns the two picked row indices in selection order and true
// when exactly two rows are marked (ready to diff).
func (t *HistoryTable) PickedVersions() (int, int, bool) {
	if len(t.picks) != comparePickCount {
		return 0, 0, false
	}

	return t.picks[0], t.picks[1], true
}

// RowAtLine maps a 0-based content line to a row index, or (0, false) when the
// line is past the last visible row.
func (t *HistoryTable) RowAtLine(line int) (int, bool) {
	if line < 0 || len(t.rows) == 0 {
		return 0, false
	}

	visible := t.visibleRows()

	// Each entry occupies one line, plus one for its tag line when present.
	row := t.offset
	consumed := 0

	for row < len(t.rows) && consumed < visible {
		if consumed == line {
			return row, true
		}

		consumed++

		if t.rows[row].TagsLine != "" {
			if consumed == line {
				return row, true
			}

			consumed++
		}

		row++
	}

	return 0, false
}

// View renders the table body into width×height.
func (t *HistoryTable) View() string {
	if t.height <= 0 || t.width <= 0 {
		return ""
	}

	visible := t.visibleRows()
	lines := make([]string, 0, t.height)
	row := t.offset

	for row < len(t.rows) && len(lines) < visible {
		lines = append(lines, t.renderRow(row))

		if tags := t.rows[row].TagsLine; tags != "" && len(lines) < visible {
			lines = append(lines, t.styles.PageHint.Render(truncate("     "+tags, t.width)))
		}

		row++
	}

	for len(lines) < t.height {
		lines = append(lines, "")
	}

	return strings.Join(lines[:t.height], "\n")
}

// renderRow renders one version row: a cursor/compare marker, the version label
// and date, the current marker, and any badges.
func (t *HistoryTable) renderRow(idx int) string {
	row := t.rows[idx]

	marker := "  "

	switch {
	case t.compare && indexOf(t.picks, idx) >= 0:
		marker = t.styles.StatusValue.Render("◉ ")
	case idx == t.selected:
		marker = t.styles.StatusValue.Render("▸ ")
	}

	label := row.Label
	if idx == t.selected {
		label = t.styles.StatusValue.Render(label)
	}

	parts := []string{marker + label}
	if row.Date != "" {
		parts = append(parts, row.Date)
	}

	if row.Current {
		parts = append(parts, t.styles.PageHint.Render("current"))
	}

	if badges := renderBadges(t.styles, row.Badges); badges != "" {
		parts = append(parts, badges)
	}

	return truncate(strings.Join(parts, "  "), t.width)
}

func (t *HistoryTable) visibleRows() int { return t.height }

// maxOffset caps scrolling so the last row stays reachable.
func (t *HistoryTable) maxOffset() int {
	return max(len(t.rows)-t.visibleRows(), 0)
}

// ensureVisible keeps the selection on-screen.
func (t *HistoryTable) ensureVisible() {
	visible := t.visibleRows()
	if visible <= 0 {
		t.offset = 0

		return
	}

	if t.selected < t.offset {
		t.offset = t.selected
	}

	if t.selected >= t.offset+visible {
		t.offset = t.selected - visible + 1
	}

	t.offset = clamp(t.offset, 0, t.maxOffset())
}

// indexOf returns the position of v in xs, or -1.
func indexOf(xs []int, v int) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}

	return -1
}
