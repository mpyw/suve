//declscope:namespace list
//
// The EntryList half of the scroll-report contract tests (Move/Scroll/
// SelectIndex report whether the offset changed); the HistoryTable mirror
// lives in scroll_report_history_internal_test.go. Each half joins its
// subject's namespace because it reads the widget's offset directly.

package components

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/tui/styles"
)

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
	l.SetRows(scrollReportListRows(12), false)
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
	l.SetRows(scrollReportListRows(12), false)
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
	l.SetRows(scrollReportListRows(12), false)
	l.SetSize(40, 4)

	assert.False(t, l.SelectIndex(0), "selecting the already-visible top row does not scroll")
	assert.True(t, l.SelectIndex(11), "selecting the last row scrolls it into view")
}
