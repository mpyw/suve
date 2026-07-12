package browser

import (
	tea "charm.land/bubbletea/v2"
)

// handleMouseClick resolves a left click against the last-rendered hit map and
// reduces it to the SAME internal action its keyboard equivalent performs: a list
// row click selects that row and loads its detail (like a key move); a history
// row click focuses/selects it and, in compare mode, picks it (opening the diff
// once two rows are picked, like enter); the value label toggles the mask (like
// x); and each header region focuses a field or toggles/refreshes (like p / / /
// v / r / ⟳).
func (m *Model) handleMouseClick(msg tea.MouseClickMsg) (*Model, tea.Cmd) {
	// A mouse interaction dismisses the transient invalid-action status, matching
	// the key path and the staging page.
	m.actionStatus = ""

	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	id, _, dy, ok := m.hits.At(msg.X, msg.Y)
	if !ok {
		return m, nil
	}

	switch id {
	case regionList:
		if idx, rowOK := m.list.RowAtLine(dy); rowOK {
			m.focus = focusList
			m.list.SelectIndex(idx)

			return m, m.selectionCmd()
		}
	case regionHistory:
		if idx, rowOK := m.history.RowAtLine(dy); rowOK {
			return m, m.clickHistoryRow(idx)
		}
	case regionValueLabel:
		m.valuePane.ToggleMask()
	case regionNamespace:
		return m, m.cycleNamespace()
	case regionPrefix:
		m.focusPrefix()
	case regionFilter:
		m.focusFilter()
	case regionValues:
		return m, m.toggleValues()
	case regionRecursive:
		return m, m.toggleRecursive()
	case regionRefresh:
		return m, m.refreshList()
	}

	return m, nil
}

// clickHistoryRow focuses and selects a history row; in compare mode it picks the
// row and, once two rows are picked, opens the diff — the click counterpart of the
// keyboard space-pick / enter-diff flow, reducing to the same nav.OpenDiff.
func (m *Model) clickHistoryRow(idx int) tea.Cmd {
	m.focus = focusHistory
	m.history.SelectIndex(idx)

	if !m.history.Compare() {
		return nil
	}

	m.history.TogglePick()

	if _, _, ok := m.history.PickedVersions(); ok {
		return m.openDiff()
	}

	return nil
}

// handleMouseWheel routes a wheel to the region under the pointer: the list, the
// history, or the value pane (a wheel anywhere else in the detail also scrolls the
// value pane). A wheel over the header or off the page is dropped — it never falls
// through to the value pane.
func (m *Model) handleMouseWheel(msg tea.MouseWheelMsg) (*Model, tea.Cmd) {
	delta := wheelDelta(msg.Button)
	if delta == 0 {
		return m, nil
	}

	id, _, _, ok := m.hits.At(msg.X, msg.Y)
	if !ok {
		return m, nil
	}

	switch id {
	case regionList:
		m.list.Scroll(delta)
	case regionHistory:
		m.history.Scroll(delta)
	case regionDetail, regionValueLabel:
		return m, m.valuePane.Update(msg)
	}

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
