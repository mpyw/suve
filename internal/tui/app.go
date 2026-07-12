// Package tui implements suve's terminal UI — a third frontend beside the CLI
// and the Wails GUI. It is pure Go and untagged, so it ships in the default CLI
// build. Like the GUI it consumes internal/usecase/* over the provider Registry
// and the neutral internal/capability matrix; unlike the GUI, provider and
// scope are fixed at launch (no in-app switching). This file holds the root
// model — the app shell that owns the status bar, tab bar, help bar, and the
// page and dialog stacks, and dispatches every message in the order
// dialogs → global keys → active page.
package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/styles"
)

// Layout row heights for the fixed chrome around the page body.
const (
	statusBarHeight = 1
	tabBarHeight    = 1
	separatorHeight = 1

	// Minimum usable terminal size; below this the shell shows a notice instead
	// of a cramped, unreadable layout.
	minWidth  = 60
	minHeight = 16
)

// identityFetcher resolves the AWS caller identity for the status bar. It takes
// no context: the launch layer builds it as a closure over the Run context, so
// the model stays free of both a stored context and the AWS provider package.
type identityFetcher func() (components.AWSIdentity, error)

// config is the constructor input for the root model.
type config struct {
	// scope is the fixed provider scope the TUI was launched with.
	scope provider.Scope
	// service preselects the initial tab ("param"/"secret", or "").
	service string
	// fetchIdentity, when non-nil and the scope is AWS, is run asynchronously on
	// Init to fill the status bar's account/region/profile.
	fetchIdentity identityFetcher
	// identity, when non-nil, seeds the AWS identity directly (used by tests and
	// any already-resolved launch), bypassing the async fetch.
	identity *components.AWSIdentity
}

// dialog is a modal overlay in the app shell's dialog stack. While any dialog
// is open it consumes input first (modality); Esc closes the top one. Concrete
// dialogs (entry form, confirm, results, error) land in later steps.
type dialog interface {
	// Update handles a forwarded message and returns the (possibly replaced)
	// dialog plus any command.
	Update(tea.Msg) (dialog, tea.Cmd)
	// View renders the dialog box content (the app frames and centers it).
	View() string
}

// awsIdentityMsg carries a resolved AWS identity back to the model.
type awsIdentityMsg struct{ id components.AWSIdentity }

// awsIdentityErrMsg reports that the AWS identity lookup failed; the status bar
// simply stops showing the loading placeholder.
type awsIdentityErrMsg struct{ err error }

// App is the root Bubble Tea model — the app shell.
type App struct {
	width  int
	height int

	scope   provider.Scope
	service string

	tabs      []components.Tab
	activeTab int

	// pages is the page stack; the top is the active page. dialogs is the modal
	// overlay stack; the top dialog captures input while any dialog is open.
	pages   []page
	dialogs []dialog

	keys   keys.Map
	styles styles.Styles
	help   help.Model

	fetchIdentity   identityFetcher
	identity        *components.AWSIdentity
	identityLoading bool
}

// newApp builds the root model from a launch config: it derives the tab set
// from the capability matrix (scope-gated), preselects the launch tab, and
// seeds the page stack with that tab's placeholder.
func newApp(cfg config) *App {
	st := styles.New()
	tabs := buildTabs(cfg.scope)
	active := initialTabIndex(tabs, cfg.service)

	m := &App{
		scope:         cfg.scope,
		service:       cfg.service,
		tabs:          tabs,
		activeTab:     active,
		keys:          keys.Default(),
		styles:        st,
		help:          help.New(),
		fetchIdentity: cfg.fetchIdentity,
		identity:      cfg.identity,
	}

	m.identityLoading = cfg.scope.Provider == provider.ProviderAWS &&
		cfg.identity == nil && cfg.fetchIdentity != nil

	if len(tabs) > 0 {
		m.pages = []page{m.pageForTab(active)}
	} else {
		m.pages = []page{newPlaceholderPage(st, "", "no services available for this scope")}
	}

	return m
}

// Init kicks off the async AWS identity fetch (AWS scope only).
func (m *App) Init() tea.Cmd {
	if m.identityLoading {
		return m.fetchIdentityCmd()
	}

	return nil
}

// fetchIdentityCmd runs the injected identity fetcher off the update loop.
func (m *App) fetchIdentityCmd() tea.Cmd {
	fetch := m.fetchIdentity

	return func() tea.Msg {
		id, err := fetch()
		if err != nil {
			return awsIdentityErrMsg{err: err}
		}

		return awsIdentityMsg{id: id}
	}
}

// Update dispatches messages. Input (keys, mouse) is routed dialogs-first, then
// through the global key map, then to the active page. Window resizes fan out
// to the active page and every dialog; async identity results update the status
// bar.
func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

		return m, m.forwardResize(msg)
	case awsIdentityMsg:
		id := msg.id
		m.identity = &id
		m.identityLoading = false

		return m, nil
	case awsIdentityErrMsg:
		m.identityLoading = false

		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	default:
		return m.routeToFocused(msg)
	}
}

// handleKey applies the dialogs → global keys → active page order for a key
// press.
func (m *App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Modal: a dialog on top consumes input first; Esc closes it.
	if len(m.dialogs) > 0 {
		if key.Matches(msg, m.keys.Back) {
			m.popDialog()

			return m, nil
		}

		return m.updateTopDialog(msg)
	}

	// Numbered tab jumps (1/2/3) map to a tab index directly.
	if i, ok := numberedTabJump(m.keys, msg); ok {
		m.jumpTab(i)

		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll

		return m, nil
	case key.Matches(msg, m.keys.NextTab):
		m.cycleTab(1)

		return m, nil
	case key.Matches(msg, m.keys.PrevTab):
		m.cycleTab(-1)

		return m, nil
	case key.Matches(msg, m.keys.Copy):
		return m, copyToClipboard(m.copyText())
	}

	return m.updateActivePage(msg)
}

// numberedTabJump maps a 1/2/3 key press to its zero-based tab index. The index
// comes from the binding's position, so there are no magic tab numbers.
func numberedTabJump(k keys.Map, msg tea.KeyPressMsg) (int, bool) {
	for i, binding := range []key.Binding{k.Tab1, k.Tab2, k.Tab3} {
		if key.Matches(msg, binding) {
			return i, true
		}
	}

	return 0, false
}

// handleMouseClick routes a left click: to the top dialog when modal, else to
// the tab bar (a tab click switches tabs, reducing to the same tab-select the
// jump keys perform), else to the active page.
func (m *App) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if len(m.dialogs) > 0 {
		return m.updateTopDialog(msg)
	}

	if msg.Button == tea.MouseLeft && msg.Y == m.tabBarRow() {
		if i, ok := m.tabBar().TabAtX(msg.X); ok {
			m.jumpTab(i)

			return m, nil
		}
	}

	return m.updateActivePage(msg)
}

// handleMouseWheel routes a wheel event to the top dialog when modal, else to
// the active page (pane scrolling lands with the real pages).
func (m *App) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if len(m.dialogs) > 0 {
		return m.updateTopDialog(msg)
	}

	return m.updateActivePage(msg)
}

// routeToFocused forwards a non-input message to the focused component: the top
// dialog when modal, else the active page.
func (m *App) routeToFocused(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.dialogs) > 0 {
		return m.updateTopDialog(msg)
	}

	return m.updateActivePage(msg)
}

// updateActivePage forwards a message to the top page.
func (m *App) updateActivePage(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.pages) == 0 {
		return m, nil
	}

	top := len(m.pages) - 1
	p, cmd := m.pages[top].Update(msg)
	m.pages[top] = p

	return m, cmd
}

// updateTopDialog forwards a message to the top dialog.
func (m *App) updateTopDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	top := len(m.dialogs) - 1
	d, cmd := m.dialogs[top].Update(msg)
	m.dialogs[top] = d

	return m, cmd
}

// forwardResize fans a window-size message out to the active page and every
// dialog so each can recompute its own layout.
func (m *App) forwardResize(msg tea.WindowSizeMsg) tea.Cmd {
	var cmds []tea.Cmd

	if len(m.pages) > 0 {
		top := len(m.pages) - 1

		p, cmd := m.pages[top].Update(msg)
		m.pages[top] = p

		cmds = append(cmds, cmd)
	}

	for i := range m.dialogs {
		d, cmd := m.dialogs[i].Update(msg)
		m.dialogs[i] = d

		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

// popDialog removes the top dialog, if any.
func (m *App) popDialog() {
	if len(m.dialogs) > 0 {
		m.dialogs = m.dialogs[:len(m.dialogs)-1]
	}
}

// cycleTab moves the active tab by delta, wrapping around the ends.
func (m *App) cycleTab(delta int) {
	n := len(m.tabs)
	if n == 0 {
		return
	}

	m.setTab(((m.activeTab+delta)%n + n) % n)
}

// jumpTab selects tab i directly, ignoring an index past the last tab (so "3"
// with two tabs is a no-op rather than snapping to the last).
func (m *App) jumpTab(i int) {
	if i < 0 || i >= len(m.tabs) {
		return
	}

	m.setTab(i)
}

// setTab switches to a valid tab index and swaps in that tab's page.
func (m *App) setTab(i int) {
	if i == m.activeTab {
		return
	}

	m.activeTab = i
	m.setActivePage(m.pageForTab(i))
}

// pageForTab builds the placeholder page for a tab index.
func (m *App) pageForTab(i int) page {
	tab := m.tabs[i]

	return newPlaceholderPage(m.styles, tab.Title, placeholderNotice(tab))
}

// setActivePage replaces the top page on the stack.
func (m *App) setActivePage(p page) {
	if len(m.pages) == 0 {
		m.pages = []page{p}

		return
	}

	m.pages[len(m.pages)-1] = p
}

// copyText is the value the `y` key copies. The skeleton has no focused value
// yet (real pages supply one from Step 3), so it copies an empty string; the
// wiring — key → OSC52 command — is what this step lands.
func (m *App) copyText() string {
	return ""
}

// tabBarRow is the terminal row the tab bar renders on (0-based), used to
// hit-test tab clicks. It sits directly under the single-line status bar.
func (m *App) tabBarRow() int {
	return statusBarHeight
}

// statusBar builds the status-bar component for the current state.
func (m *App) statusBar() components.StatusBar {
	return components.StatusBar{
		Scope:    m.scope,
		Styles:   m.styles,
		Identity: m.identity,
		Loading:  m.identityLoading,
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

	if m.width <= 0 || m.height <= 0 {
		return view
	}

	if m.width < minWidth || m.height < minHeight {
		view.SetContent(m.renderTooSmall())

		return view
	}

	view.SetContent(m.render())

	return view
}

// renderTooSmall shows the minimum-size notice.
func (m *App) renderTooSmall() string {
	notice := m.styles.PageHint.Render("terminal too small")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, notice)
}

// render composes status bar / tab bar / separator / page body / help bar, and
// overlays any dialogs as lipgloss layers.
func (m *App) render() string {
	status := m.statusBar().View(m.width)
	tabbar := m.tabBar().View(m.width)
	separator := m.styles.Separator.Render(strings.Repeat("─", m.width))
	helpBar := m.helpView()

	chrome := statusBarHeight + tabBarHeight + separatorHeight + lipgloss.Height(helpBar)
	pageHeight := max(m.height-chrome, 0)

	var body string
	if len(m.pages) > 0 {
		body = m.pages[len(m.pages)-1].View(m.width, pageHeight)
	}

	body = lipgloss.NewStyle().Width(m.width).Height(pageHeight).Render(body)

	base := strings.Join([]string{status, tabbar, separator, body, helpBar}, "\n")

	if len(m.dialogs) == 0 {
		return base
	}

	return m.overlayDialogs(base)
}

// helpView renders the bottom help bar (short by default, full when toggled).
func (m *App) helpView() string {
	return m.styles.HelpBar.Render(" " + m.help.View(m.keys))
}

// overlayDialogs draws each dialog as a centered lipgloss v2 layer over the
// page, later dialogs on top.
func (m *App) overlayDialogs(base string) string {
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewLayer(base))

	for _, d := range m.dialogs {
		box := m.styles.Dialog.Render(d.View())
		x := max((m.width-lipgloss.Width(box))/2, 0)   //nolint:mnd // centered horizontally
		y := max((m.height-lipgloss.Height(box))/2, 0) //nolint:mnd // centered vertically
		canvas.Compose(lipgloss.NewLayer(box).X(x).Y(y))
	}

	return canvas.Render()
}

// placeholderNotice is the muted text a tab's placeholder page shows, naming
// the step its real page arrives in.
func placeholderNotice(t components.Tab) string {
	if t.Service == stagingService {
		return "(staging page lands in Step 5)"
	}

	return "(browser page lands in Step 3)"
}
