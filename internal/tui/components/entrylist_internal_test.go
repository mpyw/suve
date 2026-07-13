package components

import (
	"strings"
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
	l.SetRows(rows, false)
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
	}, false)
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
	l.SetRows(valuedListRows(3), false)
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
	l.SetRows(valuedListRows(8), false)
	l.SetSize(40, 5) // fits ~2 rows

	last := l.Len() - 1
	l.SelectIndex(last)

	assert.Equal(t, len(l.rowLines(last)), listWindowRowSpan(&l, last),
		"selecting the last row must show all of its rendered lines")
}

// TestEntryListLoadMoreReservesFooterLine pins that the load-more footer still
// takes the last visible line when a next page is reported, with two-line rows.
func TestEntryListLoadMoreReservesFooterLine(t *testing.T) {
	t.Parallel()

	l := NewEntryList(styles.New())
	l.SetRows(valuedListRows(8), true) // hasMore
	l.SetSize(40, 5)

	view := l.View()
	assert.Contains(t, view, "load more (L)", "the footer renders when there is a next page")
	assert.Len(t, strings.Split(view, "\n"), l.height, "the view is padded to exactly the pane height")
}
