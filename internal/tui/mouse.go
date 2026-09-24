//declscope:namespace app
//
// Mouse routing for the App: clicks, wheel, motion and release, translated
// from screen rows to the dialog or page under the pointer.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// handleMouseClick routes a left click: to the top dialog when modal, else to
// the tab bar (a tab click switches tabs, reducing to the same tab-select the
// jump keys perform), else to the active page.
func (m *App) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if len(m.dialogs) > 0 {
		return m.forwardDialogClick(msg)
	}

	// Below the minimum size the shell draws only the too-small notice — no tab
	// bar or help bar — so a click on those rows must not hit-test invisible
	// chrome. Gate mouse chrome selection on the same size the shell renders.
	if m.width < minWidth || m.height < minHeight {
		return m.updateActivePage(m.translateMouseClick(msg))
	}

	if msg.Button == tea.MouseLeft && msg.Y == m.tabBarRow() {
		if i, ok := m.tabBar().TabAtX(msg.X); ok {
			return m, m.jumpTab(i)
		}
	}

	// A click on the help bar toggles the short/full help, the same as `?`.
	if msg.Button == tea.MouseLeft && msg.Y >= m.helpBarTop() {
		m.help.ShowAll = !m.help.ShowAll

		return m, nil
	}

	return m.updateActivePage(m.translateMouseClick(msg))
}

// forwardDialogClick resolves a modal click against the overlay hit map and, when
// it lands on the top dialog's box, forwards it translated into that dialog's
// content-local coordinates (box origin from the hit region's bounds, plus the
// frame inset). A click outside the top box is swallowed — a modal never leaks a
// click to the page beneath it.
func (m *App) forwardDialogClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	top := len(m.dialogs) - 1

	id, dx, dy, ok := m.dialogHits.At(msg.X, msg.Y)
	if !ok || id != strconv.Itoa(top) {
		return m, nil
	}

	left, topInset := m.dialogFrameOffset()
	msg.X = dx - left
	msg.Y = dy - topInset

	return m.updateTopDialog(msg)
}

// helpBarTop is the first terminal row the (short or full) help bar occupies, so
// a click at or below it is a help-toggle rather than a page-body click.
func (m *App) helpBarTop() int {
	return m.height - lipgloss.Height(m.helpView())
}

// translateMouseClick shifts a click's Y from screen coordinates into the active
// page's local coordinates (the page body sits below the fixed chrome), so a
// page hit-tests its own layout without knowing the shell's row offsets.
func (m *App) translateMouseClick(msg tea.MouseClickMsg) tea.MouseClickMsg {
	msg.Y -= m.pageBodyTop()

	return msg
}

// pageBodyTop is the screen row the active page's body starts on (below the
// status bar, tab bar, and separator).
func (m *App) pageBodyTop() int {
	return statusBarHeight + tabBarHeight + separatorHeight
}

// handleMouseWheel routes a wheel event to the top dialog when modal, else to
// the active page (pane scrolling lands with the real pages).
func (m *App) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if len(m.dialogs) > 0 {
		return m.updateTopDialog(msg)
	}

	msg.Y -= m.pageBodyTop()

	return m.updateActivePage(msg)
}

// handleMouseMotion routes a mouse-motion event (reported with a button held in
// CellMotion mode) to the active page in page-local coordinates, so a page can
// follow a drag (the browser's divider resize). It is swallowed while a dialog is
// modal — a drag never leaks to the page beneath an overlay.
func (m *App) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if len(m.dialogs) > 0 {
		return m, nil
	}

	msg.Y -= m.pageBodyTop()

	return m.updateActivePage(msg)
}

// handleMouseRelease routes a mouse button-up to the active page in page-local
// coordinates, so a page can end a drag. It is swallowed while a dialog is modal.
func (m *App) handleMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if len(m.dialogs) > 0 {
		return m, nil
	}

	msg.Y -= m.pageBodyTop()

	return m.updateActivePage(msg)
}
