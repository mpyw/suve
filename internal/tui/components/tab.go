package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/styles"
)

// Tab is one entry in the tab bar. Service is the internal key the app uses to
// pick the page for the tab ("param", "secret", or "staging"); Title is the
// display label (e.g. "Key Vault").
type Tab struct {
	Title   string
	Service string
}

// Layout constants shared by View and TabAtX so a click maps to the exact cell
// the render drew — the click test derives coordinates from TabAtX rather than
// hard-coding them, so moving these never breaks it.
const (
	tabBarLeftPad = 1
	tabGap        = 1
)

// TabBar renders the row of tab labels and hit-tests mouse clicks against them.
type TabBar struct {
	Tabs   []Tab
	Active int
	Styles styles.Styles
}

// cells renders each tab to its styled cell, active tab highlighted.
func (b TabBar) cells() []string {
	cells := make([]string, len(b.Tabs))
	for i, t := range b.Tabs {
		style := b.Styles.TabInactive
		if i == b.Active {
			style = b.Styles.TabActive
		}

		cells[i] = style.Render(t.Title)
	}

	return cells
}

// View renders the tab bar to a single line, truncated to width.
func (b TabBar) View(width int) string {
	line := strings.Repeat(" ", tabBarLeftPad) + strings.Join(b.cells(), strings.Repeat(" ", tabGap))

	return truncate(line, width)
}

// TabAtX returns the index of the tab whose rendered cell contains display
// column x, or (0, false) when x falls in the padding between/around tabs. It
// walks the same cumulative layout View draws, so the two never disagree.
func (b TabBar) TabAtX(x int) (int, bool) {
	col := tabBarLeftPad

	for i, cell := range b.cells() {
		w := lipgloss.Width(cell)
		if x >= col && x < col+w {
			return i, true
		}

		col += w + tabGap
	}

	return 0, false
}
