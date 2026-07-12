package staging

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// handleMouseClick resolves a left click against the last-rendered hit map and
// reduces it to the SAME internal action its keyboard equivalent performs: the
// header toggle flips the diff/value view (like v); apply-all/reset-all/refresh
// mirror A/R/ctrl+r; a section's apply/reset button applies/resets that section;
// the notice dismisses (like esc); and a row click only SELECTS the row (never a
// destructive cancel — removal is `u`-only, and enter's detail stays a key
// action, resolving the browser/staging row-click divergence toward a single
// "click selects" model).
func (m *Model) handleMouseClick(msg tea.MouseClickMsg) (*Model, tea.Cmd) {
	m.status = "" // a mouse interaction dismisses the transient invalid-action status

	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	id, _, _, ok := m.hits.At(msg.X, msg.Y)
	if !ok {
		return m, nil
	}

	switch {
	case id == regionNotice:
		m.noticeDismissed = true
	case id == regionViewToggle:
		return m, m.toggleView()
	case id == regionApplyAll:
		return m, m.apply(true)
	case id == regionResetAll:
		return m, m.reset(true)
	case id == regionRefresh:
		return m, m.reload()
	case strings.HasPrefix(id, prefixSecApply):
		return m, m.clickSection(id, prefixSecApply, m.applyServices)
	case strings.HasPrefix(id, prefixSecReset):
		return m, m.clickSection(id, prefixSecReset, m.resetServices)
	case strings.HasPrefix(id, prefixRow):
		return m.selectClickedRow(id)
	}

	return m, nil
}

// clickSection resolves a section-button region to its service and calls the
// per-service apply/reset (the same reduction the header a/r keys perform).
func (m *Model) clickSection(id, prefix string, action func([]string, bool) tea.Cmd) tea.Cmd {
	i, ok := idIndex(id, prefix)
	if !ok || i >= len(m.sections) {
		return nil
	}

	return action([]string{m.sections[i].service}, false)
}

// selectClickedRow selects the clicked row (resetting the reveal like a key move,
// #694) without performing any row action — a single click only navigates.
func (m *Model) selectClickedRow(id string) (*Model, tea.Cmd) {
	row, ok := idIndex(id, prefixRow)
	if !ok || row >= len(m.rows) {
		return m, nil
	}

	m.selected = row
	m.reveal = false

	return m, nil
}

// handleMouseWheel scrolls the section body regardless of the pointer's column,
// so there is no dead zone (the staging page has a single scrollable body).
func (m *Model) handleMouseWheel(msg tea.MouseWheelMsg) (*Model, tea.Cmd) {
	m.status = "" // a mouse interaction dismisses the transient invalid-action status

	delta := wheelDelta(msg.Button)
	if delta == 0 {
		return m, nil
	}

	m.scroll += delta
	m.scrollToSelection = false // an explicit scroll is not overridden by the selection

	return m, nil
}

// wheelDelta maps a wheel button to a row delta (down positive, up negative).
func wheelDelta(button tea.MouseButton) int {
	switch button {
	case tea.MouseWheelDown:
		return 1
	case tea.MouseWheelUp:
		return -1
	default:
		return 0
	}
}
