package staging

import (
	tea "charm.land/bubbletea/v2"
)

// handleMouseClick maps a left click onto the last-rendered geometry, reducing
// each click to the SAME internal action its keyboard equivalent performs: a
// section's apply/reset button to apply/reset that section, an entry row to the
// full-diff detail (like enter), a tag row to its cancel (like enter/x), and the
// auto-unstaged notice to dismiss. It follows the browser's hand-rolled geom
// hit-testing so #663 can migrate both pages to the compositor uniformly.
func (m *Model) handleMouseClick(msg tea.MouseClickMsg) (*Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	if m.geom.noticeRow >= 0 && msg.Y == m.geom.noticeRow {
		m.noticeDismissed = true

		return m, nil
	}

	line := msg.Y - m.geom.bodyTop
	if line < 0 || line >= len(m.geom.lines) {
		return m, nil
	}

	desc := m.geom.lines[line]

	if desc.section >= 0 {
		return m.clickSection(desc, msg.X)
	}

	if desc.row >= 0 {
		return m.clickRow(desc.row)
	}

	return m, nil
}

// clickSection handles a click on a section header: apply/reset when the click
// falls on the corresponding button columns.
func (m *Model) clickSection(desc lineDesc, x int) (*Model, tea.Cmd) {
	if desc.section >= len(m.sections) {
		return m, nil
	}

	service := m.sections[desc.section].service

	switch {
	case inRange(x, desc.apply):
		return m, m.applyServices([]string{service}, false)
	case inRange(x, desc.reset):
		return m, m.resetServices([]string{service}, false)
	default:
		return m, nil
	}
}

// clickRow selects a row and performs its enter action (detail for an entry,
// cancel for a tag change), so a click reduces to the key path.
func (m *Model) clickRow(row int) (*Model, tea.Cmd) {
	if row >= len(m.rows) {
		return m, nil
	}

	m.selected = row

	return m, m.onEnter()
}

// handleMouseWheel scrolls the section body.
func (m *Model) handleMouseWheel(msg tea.MouseWheelMsg) (*Model, tea.Cmd) {
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

// inRange reports whether x is within the [start,end) column range.
func inRange(x int, r [2]int) bool {
	return x >= r[0] && x < r[1]
}
