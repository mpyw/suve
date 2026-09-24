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
	"time"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/hit"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/styles"
	"github.com/mpyw/suve/internal/tui/termquirk"
)

// cloudShellRepaintMsg drives the continuous full-repaint loop used in browser
// cloud shells (see cloudShellRepaintCmd).
type cloudShellRepaintMsg struct{}

// cloudShellRepaintInterval is how often the TUI forces a full repaint in a
// browser cloud shell. Fast enough that any transient corruption is wiped within
// a frame or two, slow enough to stay readable.
const cloudShellRepaintInterval = 120 * time.Millisecond

// cloudShellRepaintCmd schedules the next full-repaint tick. In AWS/Google/Azure
// browser cloud shells (xterm.js, often inside tmux) the terminal mishandles
// Bubble Tea's incremental cell writes and corrupts the display on any content
// change — the loading spinner, arriving data, or scrolling — where only a full
// repaint (what a manual window resize triggers) clears it. #859's per-scroll
// ClearScreen was too narrow: it never fired during loading and only on scroll.
// This drives a full repaint continuously instead. Gated to affected terminals
// so native terminals keep the optimized path.
func cloudShellRepaintCmd() tea.Cmd {
	return tea.Tick(cloudShellRepaintInterval, func(time.Time) tea.Msg {
		return cloudShellRepaintMsg{}
	})
}

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

// targetFetcher resolves a pending scope target (see provider.Target) for the
// status bar and the apply confirmation. It takes no context: the launch layer
// builds it as a closure over the Run context, so the model stays free of both a
// stored context and any provider package.
type targetFetcher func() (provider.Target, error)

// config is the constructor input for the root model.
type config struct {
	// scope is the fixed provider scope the TUI was launched with.
	scope provider.Scope
	// service preselects the initial tab ("param"/"secret", or "").
	service string
	// fetchTarget, when non-nil, is run asynchronously on Init when the scope's
	// target is pending (AWS: the STS caller identity).
	fetchTarget targetFetcher
	// target, when non-nil, seeds the resolved target directly (used by tests and
	// any already-resolved launch), bypassing the async fetch.
	target *provider.Target
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

// escInterceptor is a dialog that wants to own the Back (Esc) key rather than be
// bare-popped — the create/edit/tag forms' discard guard (#790). The shell
// forwards Esc into such a dialog's Update when interceptEsc reports true; the
// dialog then arms a discard confirmation (dirty) or emits CanceledMsg (clean).
// dialogAdapter forwards this to the wrapped dialogs.EscInterceptor.
type escInterceptor interface {
	interceptEsc() bool
}

// targetMsg carries a resolved scope target back to the model.
type targetMsg struct{ target provider.Target }

// targetErrMsg reports that the target lookup failed; the status bar simply
// stops showing the loading placeholder.
type targetErrMsg struct{ err error }

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

	// dialogHits is the last-rendered overlay hit map: one region per dialog box at
	// its centered screen position. A click while modal is resolved against it, and
	// the hit region's bounds give the box origin the click is translated by — so a
	// click reaching a dialog is un-offset via the compositor's layer origin rather
	// than by re-deriving the centering math (the #663 dialog-offset fix).
	dialogHits *hit.Map

	keys   keys.Map
	styles styles.Styles
	help   help.Model

	// fetchTarget resolves target while target.Pending is set.
	fetchTarget targetFetcher
	target      provider.Target

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

	// pageGen is a monotonic page-generation counter. Each browser page built by
	// pageForTab is stamped with the next value so a superseded prior page's
	// in-flight response — whose per-Model seq resets and can collide with the new
	// page's — is dropped rather than spliced into the new tab (#746).
	pageGen int
}

// newApp builds the root model from a launch config: it derives the tab set
// from the capability matrix (scope-gated), preselects the launch tab, and
// seeds the page stack with that tab's placeholder.
func newApp(cfg config) *App {
	st := styles.New()
	tabs := buildTabs(cfg.scope)
	active := initialTabIndex(tabs, cfg.service)

	m := &App{
		scope:        cfg.scope,
		service:      cfg.service,
		tabs:         tabs,
		activeTab:    active,
		keys:         keys.Default(),
		styles:       st,
		help:         help.New(),
		fetchTarget:  cfg.fetchTarget,
		target:       cfg.scope.Target(),
		sourceFor:    cfg.sourceFor,
		mutatorFor:   cfg.mutatorFor,
		stagingFor:   cfg.stagingFor,
		runCtx:       cmp.Or(cfg.runCtx, context.Background()),
		stagedCounts: map[string]int{},
	}

	if cfg.target != nil {
		m.target = *cfg.target
	}

	// Without a fetcher a pending target never resolves, so it is shown as is.
	m.target.Pending = m.target.Pending && m.fetchTarget != nil

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

// Init kicks off the async target fetch (when the target is pending) and the
// initial page's own loads.
func (m *App) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.target.Pending {
		cmds = append(cmds, m.fetchTargetCmd())
	}

	if cmd := m.initialPageCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	if termquirk.ScrollNeedsFullRepaint() {
		cmds = append(cmds, cloudShellRepaintCmd())
	}

	return tea.Batch(cmds...)
}

// fetchTargetCmd runs the injected target fetcher off the update loop.
func (m *App) fetchTargetCmd() tea.Cmd {
	fetch := m.fetchTarget

	return func() tea.Msg {
		target, err := fetch()
		if err != nil {
			return targetErrMsg{err: err}
		}

		return targetMsg{target: target}
	}
}

// Update dispatches messages. Input (keys, mouse) is routed dialogs-first, then
// through the global key map, then to the active page. Window resizes fan out
// to the active page and every dialog; async target results update the status
// bar.
func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cloudShellRepaintMsg:
		// Continuous full repaint for browser cloud shells: clear the screen and
		// arm the next tick. The tick is a 120ms timer, so batching (concurrent)
		// vs sequencing is immaterial to ordering here.
		return m, tea.Batch(tea.ClearScreen, cloudShellRepaintCmd())
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Give the help bar the terminal width so it can self-truncate its short
		// help with an ellipsis rather than overflowing once a page contributes
		// its own bindings (the latent overflow the static, width-less help had).
		m.help.SetWidth(msg.Width)

		return m, m.forwardResize(msg)
	case targetMsg:
		m.target = msg.target
		m.target.Pending = false

		return m, nil
	case targetErrMsg:
		m.target.Pending = false

		return m, nil
	case cursor.BlinkMsg:
		// Swallow the embedded text inputs' virtual-cursor blink so the focused
		// field's caret stays steadily rendered: the app draws the real terminal
		// cursor at that caret (see App.View / screenCursor), which it locates by the
		// field's steady virtual-cursor cell. Dropping the blink keeps that cell —
		// and thus the caret position — present every frame instead of flickering
		// off, and the real cursor still blinks natively (#765).
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case tea.MouseMotionMsg:
		return m.handleMouseMotion(msg)
	case tea.MouseReleaseMsg:
		return m.handleMouseRelease(msg)
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
			top := len(m.dialogs) - 1

			// A free-text form guards against an accidental discard: it owns Esc
			// itself (a dirty form arms a "press esc again" confirmation, a clean
			// form closes) rather than being bare-popped (#790). Forwarding the Esc
			// lets the form decide; it emits CanceledMsg when it actually closes.
			if g, ok := m.dialogs[top].(escInterceptor); ok && g.interceptEsc() {
				return m.updateTopDialog(msg)
			}

			// A dialog that has already mutated (the apply results view) closes on
			// Back with the same reload+voice as enter, so the staging page and its
			// badge refresh; the returned command emits MutationDoneMsg, which
			// onMutationDone turns into the single pop+reload+voice. Every other
			// dialog is bare-dismissed.
			if d, ok := m.dialogs[top].(dialogs.DismissReloader); ok {
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
// (the browser returns its value pane's raw value WITHOUT changing the mask —
// copying never reveals, #689) wins; otherwise the app's own copyValue is used
// (empty in the skeleton, so the copy is a guarded no-op).
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
