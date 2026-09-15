//declscope:namespace history
//
// The HistoryTable half of the scroll-report contract tests; the EntryList
// half lives in scroll_report_list_internal_test.go. Each half joins its
// subject's namespace because it reads the widget's offset directly.

package components

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/tui/styles"
)

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
