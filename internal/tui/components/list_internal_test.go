//declscope:namespace list
//
// In-package tests of the entry list in list.go (the file-stem namespace
// would be listInternal, which names no unit of its own).

package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/styles"
)

// listWindowRowSpan returns the number of rendered lines the window currently
// draws for row idx, derived from window()'s parallel rowOf slice — the ground
// truth for "how much of a row is on-screen" without hard-coding line offsets.
func listWindowRowSpan(l *EntryList, idx int) int {
	_, rowOf := l.window()

	n := 0

	for _, r := range rowOf {
		if r == idx {
			n++
		}
	}

	return n
}

// valuedListRows builds n rows that each carry a value preview, so each renders
// as two lines (name header + indented value line).
func valuedListRows(n int) []ListRow {
	rows := make([]ListRow, n)
	for i := range rows {
		rows[i] = ListRow{
			Name:    "/app/key-" + string(rune('A'+i)),
			Preview: "value-" + string(rune('A'+i)),
		}
	}

	return rows
}

// TestEntryListValuesOffSingleLine pins that with no preview each row is a single
// line and the classic rows-minus-height offset math applies.
func TestEntryListValuesOffSingleLine(t *testing.T) {
	t.Parallel()

	rows := make([]ListRow, 6)
	for i := range rows {
		rows[i] = ListRow{Name: "/app/key-" + string(rune('A'+i))}
	}

	l := NewEntryList(styles.New())
	l.SetRows(rows)
	l.SetSize(40, 4)

	for i := range rows {
		require.Lenf(t, l.rowLines(i), 1, "values:off row %d must be a single line", i)
	}

	l.Scroll(100)
	assert.Equal(t, l.Len()-l.height, l.maxOffset(), "single-line rows: maxOffset is rows minus height")
}

// TestEntryListValuesOnTwoLines pins the #734 layout: a row with a value preview
// renders as two lines — the name header and an indented value line beneath it.
func TestEntryListValuesOnTwoLines(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows([]ListRow{
		{Name: "/app/db", Preview: "postgres://db.internal:5432/app", Badges: []string{"staged"}},
	})
	l.SetSize(60, 6)

	lines := l.rowLines(0)
	require.Len(t, lines, 2, "a valued row renders as name header + value line")
	assert.Contains(t, lines[0], "/app/db", "the name is on the header line")
	assert.Contains(t, lines[0], "[staged]", "the badge stays on the header line, uncollided")
	assert.NotContains(t, lines[0], "postgres", "the value is NOT on the name line")
	assert.Contains(t, lines[1], "     postgres://db.internal:5432/app",
		"the value is on its own second line, indented under the header")
}

// TestEntryListRowAtLineMapsBothLines pins that a click on either line of a
// two-line valued row selects that row (the mouse hit-map honors multi-line rows).
func TestEntryListRowAtLineMapsBothLines(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows(valuedListRows(3))
	l.SetSize(40, 6)

	// Rows render as: line0=name0, line1=value0, line2=name1, line3=value1, ...
	got := map[int]int{}

	for line := range 6 {
		if idx, ok := l.RowAtLine(line); ok {
			got[line] = idx
		}
	}

	assert.Equal(t, 0, got[0], "line 0 maps to row 0 (name)")
	assert.Equal(t, 0, got[1], "line 1 maps to row 0 (value)")
	assert.Equal(t, 1, got[2], "line 2 maps to row 1 (name)")
	assert.Equal(t, 1, got[3], "line 3 maps to row 1 (value)")
}

// TestEntryListSelectLastRowFullyVisible pins that selecting the last row brings
// both of its rendered lines into a short window, even though earlier rows are
// two lines each (line-based scroll math, mirroring the history table).
func TestEntryListSelectLastRowFullyVisible(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows(valuedListRows(8))
	l.SetSize(40, 5) // fits ~2 rows

	last := l.Len() - 1
	l.SelectIndex(last)

	assert.Equal(t, len(l.rowLines(last)), listWindowRowSpan(&l, last),
		"selecting the last row must show all of its rendered lines")
}

// scrollReportListRows builds n single-line rows (no preview) with distinct names.
func scrollReportListRows(n int) []ListRow {
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
	l.SetRows(scrollReportListRows(12))
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

// TestEntryListScrollReportsChange pins that wheel Scroll reports whether the
// offset changed: a downward scroll from the top scrolls (true), and a scroll
// already clamped at an end reports no change (false) so callers skip the
// repaint.
func TestEntryListScrollReportsChange(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows(scrollReportListRows(12))
	l.SetSize(40, 4)

	assert.False(t, l.Scroll(-1), "already at top: scrolling up does not change the offset")
	assert.True(t, l.Scroll(1), "scrolling down from the top moves the offset")

	// Scroll far past the bottom, then a further down-scroll is a no-op.
	l.Scroll(100)
	assert.False(t, l.Scroll(1), "already at the bottom: scrolling down does not change the offset")
}

// TestEntryListSelectIndexReportsScroll pins that a click-select reports whether
// it scrolled a partially-visible row into view: selecting the already-visible
// row 0 does not scroll (false), selecting the last row does (true).
func TestEntryListSelectIndexReportsScroll(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows(scrollReportListRows(12))
	l.SetSize(40, 4)

	assert.False(t, l.SelectIndex(0), "selecting the already-visible top row does not scroll")
	assert.True(t, l.SelectIndex(11), "selecting the last row scrolls it into view")
}
