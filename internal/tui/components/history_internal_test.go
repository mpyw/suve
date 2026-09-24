//declscope:namespace history
//
// In-package tests of the history table in history.go (the file-stem
// namespace would be historyInternal, which names no unit of its own).

package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/styles"
)

// historyWindowRowSpan returns the number of rendered lines the window currently draws
// for row idx, derived from window()'s parallel rowOf slice. It is the ground
// truth for "how much of a row is on-screen" without hard-coding line offsets.
func historyWindowRowSpan(t *HistoryTable, idx int) int {
	_, rowOf := t.window()

	n := 0

	for _, r := range rowOf {
		if r == idx {
			n++
		}
	}

	return n
}

// historyWindowLinesForRow returns the rendered lines the window currently draws for
// row idx, in order, so a test can compare them against rowLines(idx).
func historyWindowLinesForRow(t *HistoryTable, idx int) []string {
	lines, rowOf := t.window()

	var out []string

	for i, r := range rowOf {
		if r == idx {
			out = append(out, lines[i])
		}
	}

	return out
}

// valuedHistoryRows builds n version rows that each carry a (non-secret) value, so each
// renders as two lines (header + value). Non-secret keeps reveal state out of the
// picture: the value line is present and identical whether or not the table is
// revealed.
func valuedHistoryRows(n int) []HistoryEntry {
	rows := make([]HistoryEntry, n)
	for i := range rows {
		rows[i] = HistoryEntry{
			Label: "#" + string(rune('A'+i)),
			Date:  "2026-07-13",
			Value: "value-" + string(rune('A'+i)),
		}
	}

	return rows
}

// TestHistoryTableRowLineHeights documents the multi-line-row premise the scroll
// math must honor: a valued row is two lines and a Key Vault tag row is three.
func TestHistoryTableRowLineHeights(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows([]HistoryEntry{
		{Label: "#A", Value: "v"},
		{Label: "#B", Value: "v", TagsLine: "env=prod"},
	})
	tbl.SetSize(40, 6)

	require.Len(t, tbl.rowLines(0), 2, "a valued row renders as header + value line")
	require.Len(t, tbl.rowLines(1), 3, "a valued row with a Key Vault tag renders as header + value + tag line")
}

// TestHistoryTableSelectLastRowFullyVisible pins the core #745 fix: selecting the
// last row must bring all of that row's rendered lines (header AND value) into the
// window, even though earlier rows are multi-line and the window is only 6 lines.
func TestHistoryTableSelectLastRowFullyVisible(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	rows := valuedHistoryRows(8)
	// Give the last row a Key Vault tag line so it is a 3-line row — the worst case.
	rows[len(rows)-1].TagsLine = "env=prod"
	tbl.SetRows(rows)
	tbl.SetSize(40, 6)

	last := tbl.Len() - 1
	tbl.SelectIndex(last)

	want := tbl.rowLines(last)
	got := historyWindowLinesForRow(&tbl, last)
	assert.Equal(t, want, got, "selecting the last row must show all of its rendered lines (header, value, tag)")
	assert.Len(t, got, len(want), "the selected row must be fully, not partially, visible")
}

// TestHistoryTableScrollReachesBottom pins that wheel scrolling can advance the
// offset far enough for the last row's final line to reach the window: with 2-line
// rows and a 6-line window, maxOffset must expose the whole last row, not stop 3
// rows short the way the old len(rows)-height math did.
func TestHistoryTableScrollReachesBottom(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedHistoryRows(8))
	tbl.SetSize(40, 6)

	// A big wheel delta clamps to maxOffset.
	tbl.Scroll(100)

	last := tbl.Len() - 1
	want := tbl.rowLines(last)
	got := historyWindowLinesForRow(&tbl, last)
	assert.Equal(t, want, got, "wheel-scrolling to the bottom must fully reveal the last row")

	// The offset the fix produces is strictly larger than the old rows-minus-lines
	// math (8 - 6 = 2), which is what left the bottom rows unreachable.
	assert.Greater(t, tbl.offset, tbl.Len()-tbl.height, "line-based maxOffset must exceed the old rows-minus-lines value")
}

// TestHistoryTableBottomRowsReachableHalfScreen is the direct regression for the
// filed scenario: with N valued rows and a window that fits only about half of
// them, keyboard-selecting each of the bottom rows must keep it fully visible.
func TestHistoryTableBottomRowsReachableHalfScreen(t *testing.T) {
	t.Parallel()

	const n = 10

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedHistoryRows(n)) // 20 rendered lines
	tbl.SetSize(40, 6)                // fits ~3 rows

	for i := range n {
		tbl.SelectIndex(i)
		assert.Equalf(t, len(tbl.rowLines(i)), historyWindowRowSpan(&tbl, i),
			"selecting row %d must keep all its lines visible", i)
	}
}

// TestHistoryTableKeyboardWalkKeepsSelectionVisible walks the selection down one
// row at a time (as the down key does) and asserts the selection is never drawn
// off-screen — the exact failure the issue describes.
func TestHistoryTableKeyboardWalkKeepsSelectionVisible(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedHistoryRows(9))
	tbl.SetSize(40, 6)

	for range tbl.Len() - 1 {
		tbl.Move(1)
		sel := tbl.Selected()
		assert.Equalf(t, len(tbl.rowLines(sel)), historyWindowRowSpan(&tbl, sel),
			"row %d must be fully visible after moving down onto it", sel)
	}
}

// TestHistoryTableSingleLineWheel keeps the original wheel premise green: with
// single-line rows (no value), lines == rows, and maxOffset behaves like the
// classic rows-minus-height value.
func TestHistoryTableSingleLineWheel(t *testing.T) {
	t.Parallel()

	rows := make([]HistoryEntry, 8)
	for i := range rows {
		rows[i] = HistoryEntry{Label: "#" + string(rune('A'+i))}
	}

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(rows)
	tbl.SetSize(40, 5)

	for i := range rows {
		require.Lenf(t, tbl.rowLines(i), 1, "row %d must be a single line", i)
	}

	tbl.Scroll(100)
	assert.Equal(t, tbl.Len()-tbl.height, tbl.maxOffset(), "single-line rows: maxOffset is rows minus height")
	assert.Equal(t, tbl.maxOffset(), tbl.offset, "a large wheel clamps to maxOffset")

	last := tbl.Len() - 1
	assert.Equal(t, tbl.rowLines(last), historyWindowLinesForRow(&tbl, last), "the last single-line row is reachable")
}

// TestHistoryTableMoveReportsScroll mirrors the EntryList contract for the
// version-history table.
func TestHistoryTableMoveReportsScroll(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedHistoryRows(12))
	tbl.SetSize(60, 4)

	deltas := []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}

	sawScroll, sawNoScroll := false, false

	for _, delta := range deltas {
		before := tbl.offset
		scrolled := tbl.Move(delta)

		assert.Equal(t, tbl.offset != before, scrolled, "Move must report whether the offset changed")

		if scrolled {
			sawScroll = true
		} else {
			sawNoScroll = true
		}
	}

	assert.True(t, sawScroll, "a sweep past the window must scroll at least once")
	assert.True(t, sawNoScroll, "a sweep must include in-window moves that do not scroll")
}

// TestHistoryTableMoveEmptyNoScroll pins the empty-table case.
func TestHistoryTableMoveEmptyNoScroll(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetSize(60, 4)

	assert.False(t, tbl.Move(1), "empty table must not report a scroll")
	assert.False(t, tbl.Move(-1), "empty table must not report a scroll")
}

// TestHistoryTableScrollReportsChange mirrors the EntryList wheel-scroll contract.
func TestHistoryTableScrollReportsChange(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedHistoryRows(12))
	tbl.SetSize(60, 4)

	assert.False(t, tbl.Scroll(-1), "already at top: scrolling up does not change the offset")
	assert.True(t, tbl.Scroll(1), "scrolling down from the top moves the offset")

	tbl.Scroll(100)
	assert.False(t, tbl.Scroll(1), "already at the bottom: scrolling down does not change the offset")
}

// TestHistoryTableSelectIndexReportsScroll mirrors the click-select contract.
func TestHistoryTableSelectIndexReportsScroll(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedHistoryRows(12))
	tbl.SetSize(60, 4)

	assert.False(t, tbl.SelectIndex(0), "selecting the already-visible top row does not scroll")
	assert.True(t, tbl.SelectIndex(11), "selecting the last row scrolls it into view")
}
