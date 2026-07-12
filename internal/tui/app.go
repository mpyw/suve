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
	"cmp"
	"context"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/styles"
)

// forceQuitKey is the one global escape that survives even while a page captures
// text input: ctrl+c always quits, so a focused filter can never trap the user.
//
//nolint:gochecknoglobals // immutable global escape binding
var forceQuitKey = key.NewBinding(key.WithKeys("ctrl+c"))

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
	// sourceFor builds the read source and staging probe for a service tab. It is
	// the data seam: production wires it to the registry-backed sourceFactory,
	// tests to a providermock-backed one. When nil (an uninitialized shell and
	// the staging tab), a tab shows its placeholder.
	sourceFor func(service string) (data.Source, data.StagingProbe)
	// mutatorFor builds the write-path Mutator for a service tab (the mutation
	// dialogs' seam). Production wires it to the registry-backed sourceFactory,
	// tests to a providermock-backed one; nil disables the write dialogs.
	mutatorFor func(service string) data.Mutator
	// stagingFor builds the review/apply/reset seam for a service, backing the
	// staging page's sections and the apply/reset dialogs. Production wires it to
	// the registry-backed sourceFactory, tests to a providermock-backed one; nil
	// leaves the Staging tab a placeholder.
	stagingFor func(service string) data.StagingService
	// runCtx is the Run context threaded into pages so their fetch commands are
	// cancelled when the program exits. Tests may leave it nil (newApp defaults it
	// to context.Background()).
	runCtx context.Context //nolint:containedctx // threaded into page fetch commands; mirrors the GUI
}

// dialog is a modal overlay in the app shell's dialog stack. While any dialog
// is open it consumes input first (modality); Esc closes the top one unless it
// is busy (a mutation is in flight — GUI "Modal busy" parity). Concrete dialogs
// live in internal/tui/dialogs and are adapted to this interface by
// dialogs_wire.go.
type dialog interface {
	// Update handles a forwarded message and returns the (possibly replaced)
	// dialog plus any command.
	Update(tea.Msg) (dialog, tea.Cmd)
	// View renders the dialog box content (the app frames and centers it).
	View() string
	// busy reports whether the dialog is mid-operation, so the shell suppresses
	// dismissal.
	busy() bool
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

	// copyValue is the focused value the `y` key copies. The skeleton has none
	// (real pages supply one from Step 3), so it stays empty and the copy is a
	// no-op; it is a field so a page can set it and so the empty guard is testable.
	copyValue string

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

	// sourceFor is the injected data seam (see config); runCtx is the Run context
	// threaded into pages.
	sourceFor  func(service string) (data.Source, data.StagingProbe)
	mutatorFor func(service string) data.Mutator
	stagingFor func(service string) data.StagingService
	runCtx     context.Context //nolint:containedctx // threaded into page fetch commands; mirrors the GUI

	// status is a transient one-line outcome (staged/applied/skipped/unstaged)
	// shown just above the help bar; empty renders no row.
	status string
	// stagedCounts holds the last staged-item count each service's browser
	// reported, totalled into the Staging tab's count badge.
	stagedCounts map[string]int
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
		sourceFor:     cfg.sourceFor,
		mutatorFor:    cfg.mutatorFor,
		stagingFor:    cfg.stagingFor,
		runCtx:        cmp.Or(cfg.runCtx, context.Background()),
		stagedCounts:  map[string]int{},
	}

	m.identityLoading = cfg.scope.Provider == provider.ProviderAWS &&
		cfg.identity == nil && cfg.fetchIdentity != nil

	if len(tabs) > 0 {
		p, _ := m.pageForTab(active)
		m.pages = []page{p}
	} else {
		m.pages = []page{newPlaceholderPage(st, "", "no services available for this scope")}
	}

	return m
}

// initialPageCmd returns the active page's Init command, so the initial page's
// async loads run when the program starts (Bubble Tea calls only the root Init).
func (m *App) initialPageCmd() tea.Cmd {
	if len(m.pages) == 0 {
		return nil
	}

	return initPage(m.pages[len(m.pages)-1])
}

// Init kicks off the async AWS identity fetch (AWS scope only) and the initial
// page's own loads.
func (m *App) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.identityLoading {
		cmds = append(cmds, m.fetchIdentityCmd())
	}

	if cmd := m.initialPageCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
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
	case nav.OpenStaging:
		m.openStaging()

		return m, nil
	case nav.OpenDiff:
		return m, m.pushDiff(msg)
	case nav.OpenEntryForm:
		return m, m.openEntryForm(msg)
	case nav.OpenDelete:
		return m, m.openDelete(msg)
	case nav.OpenTag:
		return m, m.openTag(msg)
	case nav.OpenRestore:
		return m, m.openRestore(msg)
	case nav.OpenApply:
		return m, m.openApply(msg)
	case nav.OpenReset:
		return m, m.openReset(msg)
	case nav.OpenStagingDetail:
		return m, m.pushStagingDetail(msg)
	case nav.OpenError:
		m.pushDialog(dialogs.NewError(m.styles, msg.Title, msg.Message), nil)

		return m, nil
	case nav.StagedCount:
		m.stagedCounts[msg.Service] = msg.Count
		m.refreshStagingTab()

		return m, nil
	case dialogs.MutationDoneMsg:
		return m, m.onMutationDone(msg)
	case dialogs.CanceledMsg:
		m.popDialog()

		return m, nil
	case nav.PopPage:
		m.popPage()

		return m, nil
	default:
		return m.routeToFocused(msg)
	}
}

// onMutationDone closes the dialog, voices the outcome, and reloads the active
// browser page (list/detail/staged badges) so the mutation is reflected.
func (m *App) onMutationDone(msg dialogs.MutationDoneMsg) tea.Cmd {
	m.popDialog()
	m.status = msg.Status

	return m.reloadActivePage()
}

// reloadActivePage asks the active page to reload after a mutation. A browser
// page reloads its list, detail, and staged badges; other pages ignore it.
func (m *App) reloadActivePage() tea.Cmd {
	if len(m.pages) == 0 {
		return nil
	}

	top := len(m.pages) - 1
	p, cmd := m.pages[top].Update(nav.Reload{})
	m.pages[top] = p

	return cmd
}

// openEntryForm builds and pushes the create/edit dialog for a service.
func (m *App) openEntryForm(req nav.OpenEntryForm) tea.Cmd {
	mut := m.mutatorForService(req.Service)
	if mut == nil {
		return nil
	}

	d, cmd := dialogs.NewEntryForm(dialogs.EntryFormInput{
		Ctx: m.runCtx, Mutator: mut, Service: req.Service, Styles: m.styles,
		Edit: req.Edit, Name: req.Name, Namespace: req.Namespace,
		Value: req.Value, TypeLabel: req.TypeLabel, Description: req.Description,
	})

	return m.pushDialog(d, cmd)
}

// openDelete builds and pushes the delete-confirm dialog for a service.
func (m *App) openDelete(req nav.OpenDelete) tea.Cmd {
	mut := m.mutatorForService(req.Service)
	if mut == nil {
		return nil
	}

	d := dialogs.NewDeleteConfirm(dialogs.DeleteInput{
		Ctx: m.runCtx, Mutator: mut, Service: req.Service, Styles: m.styles,
		Name: req.Name, Namespace: req.Namespace,
	})

	return m.pushDialog(d, nil)
}

// openTag builds and pushes the tag add/remove dialog for a service.
func (m *App) openTag(req nav.OpenTag) tea.Cmd {
	mut := m.mutatorForService(req.Service)
	if mut == nil {
		return nil
	}

	d, cmd := dialogs.NewTagForm(dialogs.TagInput{
		Ctx: m.runCtx, Mutator: mut, Service: req.Service, Styles: m.styles,
		Name: req.Name, Namespace: req.Namespace,
	})

	return m.pushDialog(d, cmd)
}

// openRestore builds and pushes the restore dialog for a service.
func (m *App) openRestore(req nav.OpenRestore) tea.Cmd {
	mut := m.mutatorForService(req.Service)
	if mut == nil {
		return nil
	}

	d, cmd := dialogs.NewRestore(dialogs.RestoreInput{
		Ctx: m.runCtx, Mutator: mut, Service: req.Service, Styles: m.styles, Name: req.Name,
	})

	return m.pushDialog(d, cmd)
}

// mutatorForService resolves the write seam for a service, or nil when none is
// wired (an uninitialized shell, or a service with no mutator).
func (m *App) mutatorForService(service string) data.Mutator {
	if m.mutatorFor == nil {
		return nil
	}

	return m.mutatorFor(service)
}

// pushDialog appends a dialog to the modal stack, seeds it with the current size
// (so its embedded form lays out before the first render), clears any transient
// status, and returns the dialog's Init command.
func (m *App) pushDialog(d dialogs.Model, initCmd tea.Cmd) tea.Cmd {
	m.status = ""
	m.dialogs = append(m.dialogs, dialogAdapter{m: d})

	if m.width > 0 && m.height > 0 {
		top := len(m.dialogs) - 1
		next, _ := m.dialogs[top].Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.dialogs[top] = next
	}

	return initCmd
}

// topDialogBusy reports whether the top dialog is mid-operation.
func (m *App) topDialogBusy() bool {
	if len(m.dialogs) == 0 {
		return false
	}

	return m.dialogs[len(m.dialogs)-1].busy()
}

// refreshStagingTab updates the Staging tab's title with the current staged
// total ("Staging" at zero, "Staging(n)" otherwise).
func (m *App) refreshStagingTab() {
	total := 0
	for _, c := range m.stagedCounts {
		total += c
	}

	for i, t := range m.tabs {
		if t.Service == stagingService {
			title := "Staging"
			if total > 0 {
				title += "(" + strconv.Itoa(total) + ")"
			}

			m.tabs[i].Title = title

			return
		}
	}
}

// openStaging switches to the Staging tab (the browser's `S` jump); setTab
// builds and loads the real staging page.
func (m *App) openStaging() {
	for i, t := range m.tabs {
		if t.Service == stagingService {
			m.setTab(i)

			return
		}
	}
}

// pushDiff pushes a diff page onto the stack for a browser compare request and
// returns its Init command.
func (m *App) pushDiff(req nav.OpenDiff) tea.Cmd {
	p := newDiffPage(m.runCtx, req, m.styles, m.keys)
	m.pages = append(m.pages, p)
	m.forwardResizeToTop()

	return p.Init()
}

// popPage pops the top page (a pushed diff), leaving the base tab page in place.
func (m *App) popPage() {
	if len(m.pages) > 1 {
		m.pages = m.pages[:len(m.pages)-1]
	}
}

// forwardResizeToTop hands the current size to the newly-pushed top page so it
// lays out before its first render.
func (m *App) forwardResizeToTop() {
	if len(m.pages) == 0 || m.width <= 0 || m.height <= 0 {
		return
	}

	chrome := statusBarHeight + tabBarHeight + separatorHeight + lipgloss.Height(m.helpView()) + m.statusLineHeight()
	pageHeight := max(m.height-chrome, 0)

	top := len(m.pages) - 1
	p, _ := m.pages[top].Update(tea.WindowSizeMsg{Width: m.width, Height: pageHeight})
	m.pages[top] = p
}

// handleKey applies the dialogs → global keys → active page order for a key
// press.
func (m *App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Modal: a dialog on top consumes input first. Only ctrl+c stays global as a
	// force-quit escape hatch — every other key (q, digits, y, ?, tab, letters)
	// is forwarded into the dialog so a focused huh text field types normally,
	// the same principle as the page capturesInput seam. Esc closes the top
	// dialog, but only when it is not busy (a mutation in flight suppresses
	// dismissal — GUI "Modal busy" parity).
	if len(m.dialogs) > 0 {
		if key.Matches(msg, forceQuitKey) {
			return m, tea.Quit
		}

		if key.Matches(msg, m.keys.Back) && !m.topDialogBusy() {
			// A dialog that has already mutated (the apply results view) closes on
			// Back with the same reload+voice as enter, so the staging page and its
			// badge refresh; the returned command emits MutationDoneMsg, which
			// onMutationDone turns into the single pop+reload+voice. Every other
			// dialog is bare-dismissed.
			if d, ok := m.dialogs[len(m.dialogs)-1].(dialogs.DismissReloader); ok {
				if cmd := d.DismissCmd(); cmd != nil {
					return m, cmd
				}
			}

			m.popDialog()

			return m, nil
		}

		return m.updateTopDialog(msg)
	}

	// A page with a focused text input (e.g. the browser's prefix/filter field)
	// owns every keystroke: the global map must not steal q/1/2/3/y/?/tab from an
	// edit. Only ctrl+c stays global, as a force-quit escape hatch.
	if m.activePageCapturesInput() {
		if key.Matches(msg, forceQuitKey) {
			return m, tea.Quit
		}

		return m.updateActivePage(msg)
	}

	// Numbered tab jumps (1/2/3) map to a tab index directly.
	if i, ok := numberedTabJump(m.keys, msg); ok {
		return m, m.jumpTab(i)
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll

		return m, nil
	case key.Matches(msg, m.keys.NextTab):
		return m, m.cycleTab(1)
	case key.Matches(msg, m.keys.PrevTab):
		return m, m.cycleTab(-1)
	case key.Matches(msg, m.keys.Copy):
		return m, m.copyFocusedValue()
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

	// Below the minimum size the shell draws only the too-small notice — no tab
	// bar — so a click at the tab-bar row must not hit-test (and switch) an
	// invisible tab. Gate mouse tab selection on the same size the shell renders.
	if m.width >= minWidth && m.height >= minHeight &&
		msg.Button == tea.MouseLeft && msg.Y == m.tabBarRow() {
		if i, ok := m.tabBar().TabAtX(msg.X); ok {
			return m, m.jumpTab(i)
		}
	}

	return m.updateActivePage(m.translateMouseClick(msg))
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

// cycleTab moves the active tab by delta, wrapping around the ends, and returns
// the new page's Init command.
func (m *App) cycleTab(delta int) tea.Cmd {
	n := len(m.tabs)
	if n == 0 {
		return nil
	}

	return m.setTab(((m.activeTab+delta)%n + n) % n)
}

// jumpTab selects tab i directly, ignoring an index past the last tab (so "3"
// with two tabs is a no-op rather than snapping to the last), and returns the
// new page's Init command.
func (m *App) jumpTab(i int) tea.Cmd {
	if i < 0 || i >= len(m.tabs) {
		return nil
	}

	return m.setTab(i)
}

// setTab switches to a valid tab index, swaps in that tab's page (resetting any
// pushed sub-page), and returns the page's Init command.
func (m *App) setTab(i int) tea.Cmd {
	if i == m.activeTab {
		return nil
	}

	m.activeTab = i

	p, cmd := m.pageForTab(i)
	m.pages = []page{p}
	m.forwardResizeToTop()

	return cmd
}

// pageForTab builds the page for a tab index: the staging page for the Staging
// tab (when its seam is wired), a browser page for a param/secret service (when
// a data source is wired), else the placeholder.
func (m *App) pageForTab(i int) (page, tea.Cmd) {
	tab := m.tabs[i]

	if tab.Service == stagingService {
		if services := m.stagingServicesFor(m.offeredServices()); len(services) > 0 {
			p := newStagingPage(m.runCtx, services, m.styles, m.keys)

			return p, p.Init()
		}

		return newPlaceholderPage(m.styles, tab.Title, placeholderNotice(tab)), nil
	}

	if m.sourceFor != nil {
		if source, staging := m.sourceFor(tab.Service); source != nil {
			p := newBrowserPage(m.runCtx, source, staging, m.styles, m.keys)

			return p, p.Init()
		}
	}

	return newPlaceholderPage(m.styles, tab.Title, placeholderNotice(tab)), nil
}

// offeredServices returns the service-axis keys the scope offers (the non-staging
// tabs, in tab order), so the staging page and the apply/reset fan-out iterate
// exactly the services that have a browser tab.
func (m *App) offeredServices() []string {
	var services []string

	for _, t := range m.tabs {
		if t.Service != stagingService {
			services = append(services, t.Service)
		}
	}

	return services
}

// stagingServicesFor resolves the staging seams for a set of service keys,
// dropping any the factory does not offer (nil seam), preserving order.
func (m *App) stagingServicesFor(services []string) []data.StagingService {
	if m.stagingFor == nil {
		return nil
	}

	out := make([]data.StagingService, 0, len(services))

	for _, s := range services {
		if svc := m.stagingFor(s); svc != nil {
			out = append(out, svc)
		}
	}

	return out
}

// openApply builds and pushes the apply confirmation for the requested services.
func (m *App) openApply(req nav.OpenApply) tea.Cmd {
	targets := m.stagingServicesFor(req.Services)
	if len(targets) == 0 {
		return nil
	}

	d := dialogs.NewApply(dialogs.ApplyInput{
		Ctx: m.runCtx, Targets: targets, TargetLine: m.applyTargetLine(),
		Title: applyTitle(req.Global, targets), EntryCount: req.EntryCount, TagCount: req.TagCount,
		Styles: m.styles,
	})

	return m.pushDialog(d, nil)
}

// openReset builds and pushes the reset confirmation for the requested services.
func (m *App) openReset(req nav.OpenReset) tea.Cmd {
	targets := m.stagingServicesFor(req.Services)
	if len(targets) == 0 {
		return nil
	}

	d := dialogs.NewReset(dialogs.ResetInput{
		Ctx: m.runCtx, Targets: targets, Title: resetTitle(req.Global, targets), Styles: m.styles,
	})

	return m.pushDialog(d, nil)
}

// pushStagingDetail pushes a full remote-vs-staged diff page for the staging
// page's `enter` detail, reusing the diff viewer over static content.
func (m *App) pushStagingDetail(req nav.OpenStagingDetail) tea.Cmd {
	p := newStaticDiffPage(data.DiffContent{
		OldLabel: req.OldLabel, NewLabel: req.NewLabel,
		OldValue: req.OldValue, NewValue: req.NewValue, Secret: req.Secret,
	}, m.styles, m.keys)
	m.pages = append(m.pages, p)
	m.forwardResizeToTop()

	return p.Init()
}

// applyTargetLine renders the apply target identity (account/region, project, or
// vault/store) shown on the apply confirmation — parity with the CLI's prompt.
func (m *App) applyTargetLine() string {
	switch m.scope.Provider {
	case provider.ProviderAWS:
		parts := []string{"aws"}
		if m.identity != nil {
			parts = appendKV(parts, "account", m.identity.Account)
			parts = appendKV(parts, "region", m.identity.Region)
		}

		return strings.Join(parts, " · ")
	case provider.ProviderGoogleCloud:
		return strings.Join(appendKV([]string{"googlecloud"}, "project", m.scope.ProjectID), " · ")
	case provider.ProviderAzure:
		parts := []string{"azure"}
		parts = appendKV(parts, "vault", m.scope.VaultName)
		parts = appendKV(parts, "store", m.scope.StoreName)

		return strings.Join(parts, " · ")
	default:
		return string(m.scope.Provider)
	}
}

// appendKV appends a "key value" segment when value is non-empty.
func appendKV(parts []string, key, value string) []string {
	if value == "" {
		return parts
	}

	return append(parts, key+" "+value)
}

// applyTitle names the apply confirmation: "— all" for the fan-out, else the
// single service's label.
func applyTitle(global bool, targets []data.StagingService) string {
	return "Apply staged changes — " + targetTitle(global, targets)
}

// resetTitle names the reset confirmation.
func resetTitle(global bool, targets []data.StagingService) string {
	return "Reset staged changes — " + targetTitle(global, targets)
}

// targetTitle is "all" for a global fan-out, else the single target's label.
func targetTitle(global bool, targets []data.StagingService) string {
	if global || len(targets) != 1 {
		return "all"
	}

	return targets[0].Label()
}

// copyFocusedValue copies the focused value through the OSC52 clipboard seam, or
// does nothing when there is no value. Copying an empty string would emit an
// OSC52 that CLEARS the user's system clipboard, so an empty copy must be a
// no-op rather than a destructive write.
func (m *App) copyFocusedValue() tea.Cmd {
	text := m.copyText()
	if text == "" {
		return nil
	}

	return copyToClipboard(text)
}

// copyText is the value the `y` key copies. A page that supplies a focused value
// (the browser reveals its masked value pane and returns the revealed value)
// wins; otherwise the app's own copyValue is used (empty in the skeleton, so the
// copy is a guarded no-op).
func (m *App) copyText() string {
	if c, ok := m.activePage().(copyable); ok {
		if text, has := c.CopyText(); has {
			return text
		}
	}

	return m.copyValue
}

// activePageCapturesInput reports whether the active page has a focused text
// input claiming raw keystrokes, so the router bypasses the global key map.
func (m *App) activePageCapturesInput() bool {
	p := m.activePage()

	return p != nil && p.capturesInput()
}

// activePage returns the top page, or nil when the stack is empty.
func (m *App) activePage() page {
	if len(m.pages) == 0 {
		return nil
	}

	return m.pages[len(m.pages)-1]
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
		return "(no staging services available for this scope)"
	}

	return "(no data source available for this service)"
}
