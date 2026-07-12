package browser

import (
	tea "charm.land/bubbletea/v2"
)

// handleMouseClick maps a left click onto the drawn geometry: a list row click
// selects that row and loads its detail (reducing to the same selection a key
// move performs), and — in compare mode — a history row click picks it.
func (m *Model) handleMouseClick(msg tea.MouseClickMsg) (*Model, tea.Cmd) {
	// A mouse interaction dismisses the transient invalid-action status, matching
	// the key path and the staging page.
	m.actionStatus = ""

	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	if line, ok := m.geom.listLine(msg.X, msg.Y); ok {
		if idx, rowOK := m.list.RowAtLine(line); rowOK {
			m.focus = focusList
			m.list.SelectIndex(idx)

			return m, m.selectionCmd()
		}
	}

	if line, ok := m.geom.historyLine(msg.X, msg.Y); ok {
		if idx, rowOK := m.history.RowAtLine(line); rowOK {
			m.focus = focusHistory
			m.history.SelectIndex(idx)

			if m.history.Compare() {
				m.history.TogglePick()
			}

			return m, nil
		}
	}

	return m, nil
}

// handleMouseWheel scrolls whichever pane the pointer is over: the list, the
// history, or (when the pointer is elsewhere in the detail) the value pane.
func (m *Model) handleMouseWheel(msg tea.MouseWheelMsg) (*Model, tea.Cmd) {
	delta := wheelDelta(msg.Button)
	if delta == 0 {
		return m, nil
	}

	switch {
	case m.geom.inList(msg.X, msg.Y):
		m.list.Scroll(delta)
	case m.geom.inHistory(msg.X, msg.Y):
		m.history.Scroll(delta)
	default:
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

// listLine maps a screen point to a 0-based list content line, or (0, false)
// when the point is outside the list content region. The right-edge bound
// (x < g.listRight) keeps the list from claiming the detail pane, which shares
// its vertical band in the two-pane layout.
func (g geom) listLine(x, y int) (int, bool) {
	if x < g.listLeft || x >= g.listRight || y < g.listTop || y >= g.listTop+g.listRows {
		return 0, false
	}

	return y - g.listTop, true
}

// historyLine maps a screen point to a 0-based history content line, bounded on
// all four sides so a point outside the drawn history content never maps in.
func (g geom) historyLine(x, y int) (int, bool) {
	if x < g.historyLeft || x >= g.historyRight || y < g.historyTop || y >= g.historyTop+g.historyRows {
		return 0, false
	}

	return y - g.historyTop, true
}

// inList reports whether a point is within the list content region.
func (g geom) inList(x, y int) bool {
	_, ok := g.listLine(x, y)

	return ok
}

// inHistory reports whether a point is within the history content region.
func (g geom) inHistory(x, y int) bool {
	_, ok := g.historyLine(x, y)

	return ok
}
