package components

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/tui/styles"
)

// singleLineRows builds n single-line rows (no preview) with distinct names.
func singleLineRows(n int) []ListRow {
	rows := make([]ListRow, n)
	for i := range rows {
		rows[i] = ListRow{Name: "/k/" + string(rune('A'+i))}
	}

	return rows
}

// TestEntryListMoveReportsScroll pins that Move returns exactly whether the
// viewport scrolled (the offset changed): a full down-then-up sweep of a list
// taller than the window must include both scrolling moves (true) and in-window
// moves (false), and the return value must agree with the offset delta on every
// step — the signal callers use to force a full repaint only on scroll.
func TestEntryListMoveReportsScroll(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows(singleLineRows(12), false)
	l.SetSize(40, 4)

	deltas := []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}

	sawScroll, sawNoScroll := false, false

	for _, delta := range deltas {
		before := l.offset
		scrolled := l.Move(delta)

		assert.Equal(t, l.offset != before, scrolled, "Move must report whether the offset changed")

		if scrolled {
			sawScroll = true
		} else {
			sawNoScroll = true
		}
	}

	assert.True(t, sawScroll, "a sweep past the window must scroll at least once")
	assert.True(t, sawNoScroll, "a sweep must include in-window moves that do not scroll")
}

// TestEntryListMoveEmptyNoScroll pins that Move on an empty list never reports a
// scroll (there is nothing to move or repaint).
func TestEntryListMoveEmptyNoScroll(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetSize(40, 4)

	assert.False(t, l.Move(1), "empty list must not report a scroll")
	assert.False(t, l.Move(-1), "empty list must not report a scroll")
}

// TestHistoryTableMoveReportsScroll mirrors the EntryList contract for the
// version-history table.
func TestHistoryTableMoveReportsScroll(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedRows(12))
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

// TestEntryListScrollReportsChange pins that wheel Scroll reports whether the
// offset changed: a downward scroll from the top scrolls (true), and a scroll
// already clamped at an end reports no change (false) so callers skip the
// repaint.
func TestEntryListScrollReportsChange(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows(singleLineRows(12), false)
	l.SetSize(40, 4)

	assert.False(t, l.Scroll(-1), "already at top: scrolling up does not change the offset")
	assert.True(t, l.Scroll(1), "scrolling down from the top moves the offset")

	// Scroll far past the bottom, then a further down-scroll is a no-op.
	l.Scroll(100)
	assert.False(t, l.Scroll(1), "already at the bottom: scrolling down does not change the offset")
}

// TestHistoryTableScrollReportsChange mirrors the EntryList wheel-scroll contract.
func TestHistoryTableScrollReportsChange(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedRows(12))
	tbl.SetSize(60, 4)

	assert.False(t, tbl.Scroll(-1), "already at top: scrolling up does not change the offset")
	assert.True(t, tbl.Scroll(1), "scrolling down from the top moves the offset")

	tbl.Scroll(100)
	assert.False(t, tbl.Scroll(1), "already at the bottom: scrolling down does not change the offset")
}

// TestEntryListSelectIndexReportsScroll pins that a click-select reports whether
// it scrolled a partially-visible row into view: selecting the already-visible
// row 0 does not scroll (false), selecting the last row does (true).
func TestEntryListSelectIndexReportsScroll(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows(singleLineRows(12), false)
	l.SetSize(40, 4)

	assert.False(t, l.SelectIndex(0), "selecting the already-visible top row does not scroll")
	assert.True(t, l.SelectIndex(11), "selecting the last row scrolls it into view")
}

// TestHistoryTableSelectIndexReportsScroll mirrors the click-select contract.
func TestHistoryTableSelectIndexReportsScroll(t *testing.T) {
	t.Parallel()

	tbl := NewHistoryTable(styles.New())
	tbl.SetRows(valuedRows(12))
	tbl.SetSize(60, 4)

	assert.False(t, tbl.SelectIndex(0), "selecting the already-visible top row does not scroll")
	assert.True(t, tbl.SelectIndex(11), "selecting the last row scrolls it into view")
}
