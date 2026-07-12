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
	// Value is this version's raw value, shown on an indented line beneath the
	// version header (masked by default when Secret is set). Empty when there is no
	// value to show, in which case no value line is drawn.
	Value string
	// Secret reports whether Value is secret material, so it is masked with bullets
	// unless the table is revealed.
	Secret bool
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
	// focused reports whether the history pane currently holds keyboard focus. The
	// selected row is drawn with the active cursor when focused and a dimmed cursor
	// when not, so the list and history never look equally selected at once.
	focused bool
	// picks holds the row indices selected for comparison (0, 1, or 2 entries).
	picks []int
	// revealed unmasks the per-version value lines for secret rows. It is driven in
	// lockstep with the detail value pane's reveal (the shared `x` toggle), so the
	// single reveal governs both the current value and the history values (GUI
	// parity), and it resets to masked whenever the rows are replaced.
	revealed bool
}

// NewHistoryTable builds an empty history table.
func NewHistoryTable(st styles.Styles) HistoryTable {
	return HistoryTable{styles: st}
}

// SetRows replaces the rows and resets scrolling/compare picks. Reveal resets to
// masked so a previous entry's reveal never carries forward onto a new one
// (mirrors the value pane resetting its mask on SetValue).
func (t *HistoryTable) SetRows(rows []HistoryEntry) {
	t.rows = rows
	t.selected = 0
	t.offset = 0
	t.picks = nil
	t.revealed = false
	t.ensureVisible()
}

// SetReveal unmasks (or re-masks) the per-version value lines. The owning page
// drives it from the detail value pane's mask state, so the shared `x` toggle
// reveals the current value and the history values together (GUI parity).
func (t *HistoryTable) SetReveal(revealed bool) { t.revealed = revealed }

// SetFocused sets whether the history pane holds keyboard focus, which selects
// the active vs dimmed selection style the next View draws with.
func (t *HistoryTable) SetFocused(focused bool) { t.focused = focused }

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
// line is past the last visible row. A row's header, value, and tag lines all
// map back to that row, so a click anywhere in a row selects it.
func (t *HistoryTable) RowAtLine(line int) (int, bool) {
	if line < 0 || len(t.rows) == 0 {
		return 0, false
	}

	_, rowOf := t.window()
	if line >= len(rowOf) {
		return 0, false
	}

	return rowOf[line], true
}

// View renders the table body into width×height.
func (t *HistoryTable) View() string {
	if t.height <= 0 || t.width <= 0 {
		return ""
	}

	lines, _ := t.window()

	for len(lines) < t.height {
		lines = append(lines, "")
	}

	return strings.Join(lines[:t.height], "\n")
}

// window renders the visible content lines for the current offset (capped at the
// visible height) and, parallel to them, the row index each line belongs to — so
// View draws them and RowAtLine maps a clicked line back to its row without the
// two drifting apart.
func (t *HistoryTable) window() (lines []string, rowOf []int) {
	visible := t.visibleRows()

	for row := t.offset; row < len(t.rows) && len(lines) < visible; row++ {
		for _, line := range t.rowLines(row) {
			if len(lines) >= visible {
				break
			}

			lines = append(lines, line)
			rowOf = append(rowOf, row)
		}
	}

	return lines, rowOf
}

// rowLines returns one row's rendered lines: the version header, an indented
// value line (masked unless revealed for a secret), and the per-version tag line
// when present. The value and tag lines share the same indent so they read as
// details of the header above them.
func (t *HistoryTable) rowLines(idx int) []string {
	row := t.rows[idx]
	lines := []string{t.renderRow(idx)}

	if value, ok := t.valueLine(row); ok {
		lines = append(lines, value)
	}

	if row.TagsLine != "" {
		lines = append(lines, t.styles.PageHint.Render(truncate("     "+row.TagsLine, t.width)))
	}

	return lines
}

// valueLine renders a row's indented value line, masked with bullets unless the
// value is non-secret or the table is revealed. Multi-line values are flattened
// onto the single line, mirroring the list preview. It returns ok=false when the
// row carries no value (an unversioned or fetch-failed entry), so no blank line
// is drawn.
func (t *HistoryTable) valueLine(row HistoryEntry) (string, bool) {
	if row.Value == "" {
		return "", false
	}

	value := flattenValue(row.Value)
	if row.Secret && !t.revealed {
		value = MaskValue(value)
	}

	return t.styles.PageHint.Render(truncate("     "+value, t.width)), true
}

// flattenValue collapses a (possibly multi-line) value onto a single line so the
// history value fits one row; newlines and tabs become single spaces.
func flattenValue(value string) string {
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ")

	return replacer.Replace(value)
}

// renderRow renders one version row: a cursor/compare marker, the version label
// and date, the current marker, and any badges.
func (t *HistoryTable) renderRow(idx int) string {
	row := t.rows[idx]

	marker := "  "

	switch {
	case t.compare && indexOf(t.picks, idx) >= 0:
		marker = t.styles.StatusValue.Render("◉ ")
	case idx == t.selected && t.focused:
		marker = t.styles.Selection.Render("▸ ")
	case idx == t.selected:
		// Selected but the history pane is idle: a hollow, dimmed cursor.
		marker = t.styles.SelectionInactive.Render("▹ ")
	}

	label := row.Label

	if idx == t.selected {
		if t.focused {
			label = t.styles.Selection.Render(label)
		} else {
			label = t.styles.SelectionInactive.Render(label)
		}
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
