//declscope:namespace app
//
// Rendering of the App: the status, tab and help bars, the active page, and
// the dialog overlay.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/hit"
	"github.com/mpyw/suve/internal/tui/keys"
)

// tabBarRow is the terminal row the tab bar renders on (0-based), used to
// hit-test tab clicks. It sits directly under the single-line status bar.
func (m *App) tabBarRow() int {
	return statusBarHeight
}

// statusBar builds the status-bar component for the current state.
func (m *App) statusBar() components.StatusBar {
	return components.StatusBar{
		Scope:  m.scope,
		Styles: m.styles,
		Target: m.target,
	}
}

// tabBar builds the tab-bar component for the current state.
func (m *App) tabBar() components.TabBar {
	return components.TabBar{
		Tabs:   m.tabs,
		Active: m.activeTab,
		Styles: m.styles,
	}
}

// View composes the shell and returns a tea.View that also carries the
// program-level toggles (alt-screen and mouse capture) Bubble Tea v2 reads from
// the returned view each frame.
func (m *App) View() tea.View {
	view := tea.NewView("")
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	// Request keyboard enhancements so an enhanced-keyboard terminal (Kitty
	// protocol) reports shift+enter distinctly from a plain Enter, which lets the
	// create/edit/tag forms bind shift+enter to submit while Enter inserts a
	// newline in the Value textarea (#791). Bubble Tea v2 always requests basic key
	// disambiguation with this set; terminals without the protocol (e.g. macOS
	// Terminal.app) collapse shift+enter to Enter, so ctrl+s is the portable submit.
	view.KeyboardEnhancements.ReportAlternateKeys = true

	if m.width <= 0 || m.height <= 0 {
		return view
	}

	if m.width < minWidth || m.height < minHeight {
		view.SetContent(m.renderTooSmall())

		return view
	}

	screen := m.render()
	view.SetContent(screen)
	// Set the real terminal cursor at the focused text field's caret so a visible
	// caret is drawn there — a bubbles text input's virtual cursor alone does not
	// surface one under AltScreen (#765). The caret is read from the composited
	// screen we just built, so it tracks the focused field wherever it is drawn: a
	// dialog's input (create/edit/tag/restore) or a page's own (the browser's
	// prefix/filter).
	view.Cursor = m.screenCursor(screen)

	return view
}

// renderTooSmall shows the minimum-size notice.
func (m *App) renderTooSmall() string {
	notice := m.styles.PageHint.Render("terminal too small")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, notice)
}

// render composes status bar / tab bar / separator / page body / status line /
// help bar, and overlays any dialogs as lipgloss layers.
func (m *App) render() string {
	status := m.statusBar().View(m.width)
	tabbar := m.tabBar().View(m.width)
	separator := m.styles.Separator.Render(strings.Repeat("─", m.width))
	helpBar := m.helpView()
	statusLine := m.statusLine()

	chrome := statusBarHeight + tabBarHeight + separatorHeight + lipgloss.Height(helpBar) + m.statusLineHeight()
	pageHeight := max(m.height-chrome, 0)

	var body string
	if len(m.pages) > 0 {
		body = m.pages[len(m.pages)-1].View(m.width, pageHeight)
	}

	body = lipgloss.NewStyle().Width(m.width).Height(pageHeight).Render(body)

	rows := []string{status, tabbar, separator, body}
	if statusLine != "" {
		rows = append(rows, statusLine)
	}

	rows = append(rows, helpBar)
	base := strings.Join(rows, "\n")

	if len(m.dialogs) == 0 {
		return base
	}

	return m.overlayDialogs(base)
}

// statusLine renders the transient outcome line (empty when there is no status).
func (m *App) statusLine() string {
	if m.status == "" {
		return ""
	}

	return m.styles.Banner.Render(" " + clipStatus(m.status, m.width))
}

// statusLineHeight is the row count the status line occupies (0 or 1).
func (m *App) statusLineHeight() int {
	if m.status == "" {
		return 0
	}

	return 1
}

// clipStatus clamps the status text to the terminal width.
func clipStatus(s string, width int) string {
	if width <= 1 || lipgloss.Width(s) <= width-1 {
		return s
	}

	return lipgloss.NewStyle().MaxWidth(width - 1).Render(s)
}

// helpView renders the bottom help bar (short by default, full when toggled).
// It composes the active page's context-aware bindings with the global shell
// keys, so the bar shows what the current page/context actually does — not just
// the shell-global tab/help/quit — which is the adaptive discoverability the
// static, global-only bar lacked (#681).
func (m *App) helpView() string {
	return m.styles.HelpBar.Render(" " + m.help.View(m.helpKeyMap()))
}

// helpKeyMap builds the help bar's key map: the active page's PageKeyMap (when
// it supplies one) layered ahead of the shell keys.
func (m *App) helpKeyMap() help.KeyMap {
	return composedHelp{page: m.activePageHelp(), shell: m.keys}
}

// activePageHelp returns the active page's context-aware PageKeyMap, or nil when
// the page supplies none (the placeholder falls back to shell-only help).
func (m *App) activePageHelp() keys.PageKeyMap {
	if h, ok := m.activePage().(helpMapper); ok {
		return h.HelpKeyMap()
	}

	return nil
}

// helpMapper is implemented by a page that supplies a context-aware PageKeyMap
// for the adaptive help bar. Checked with a type assertion (like copyable) so a
// page without one — the placeholder — cleanly falls back to shell-only help.
type helpMapper interface {
	HelpKeyMap() keys.PageKeyMap
}

// composedHelp is the help bar's key map: the active page's bindings first, then
// the always-present shell bindings (tab/help/quit). page may be nil, leaving
// just the shell keys.
type composedHelp struct {
	page  keys.PageKeyMap
	shell keys.Map
}

// ShortHelp lists the page's short bindings followed by the shell's.
func (c composedHelp) ShortHelp() []key.Binding {
	shell := c.shell.ShortHelp()
	if c.page == nil {
		return shell
	}

	page := c.page.ShortHelp()
	out := make([]key.Binding, 0, len(page)+len(shell))
	out = append(out, page...)
	out = append(out, shell...)

	return out
}

// FullHelp lists the page's full-help columns followed by the shell column.
func (c composedHelp) FullHelp() [][]key.Binding {
	shell := c.shell.FullHelp()
	if c.page == nil {
		return shell
	}

	page := c.page.FullHelp()
	out := make([][]key.Binding, 0, len(page)+len(shell))
	out = append(out, page...)
	out = append(out, shell...)

	return out
}

// overlayDialogs draws each dialog as a centered lipgloss v2 layer over the page
// (later dialogs on top) and rebuilds the overlay hit map: one region per dialog
// box at the same centered position, so a modal click is resolved and un-offset
// against the very layer that was drawn.
func (m *App) overlayDialogs(base string) string {
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewLayer(base))

	regions := make([]*lipgloss.Layer, 0, len(m.dialogs))

	for i, d := range m.dialogs {
		box := m.styles.Dialog.Render(d.View())
		x := max((m.width-lipgloss.Width(box))/2, 0)   //nolint:mnd // centered horizontally
		y := max((m.height-lipgloss.Height(box))/2, 0) //nolint:mnd // centered vertically
		canvas.Compose(lipgloss.NewLayer(box).X(x).Y(y))
		regions = append(regions,
			hit.Region(strconv.Itoa(i), x, y, lipgloss.Width(box), lipgloss.Height(box)).Z(i))
	}

	m.dialogHits = hit.New(regions...)

	return canvas.Render()
}

// dialogFrameOffset is the (left, top) content inset the shell's dialog frame
// adds around a dialog's View content: the rounded border plus horizontal
// padding. Subtracting it (plus the box origin) from a screen click yields the
// dialog's content-local coordinate.
func (m *App) dialogFrameOffset() (left, top int) {
	return m.styles.Dialog.GetBorderLeftSize() + m.styles.Dialog.GetPaddingLeft(),
		m.styles.Dialog.GetBorderTopSize() + m.styles.Dialog.GetPaddingTop()
}

// placeholderNotice is the muted text a tab's placeholder page shows, naming
// the step its real page arrives in.
func placeholderNotice(t components.Tab) string {
	if t.Service == stagingService {
		return "(no staging services available for this scope)"
	}

	return "(no data source available for this service)"
}
