//declscope:namespace app
//
// The page contract is what the App drives; this file and app.go are two
// halves of one shell.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	"context"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/pages/browser"
	"github.com/mpyw/suve/internal/tui/pages/diff"
	"github.com/mpyw/suve/internal/tui/pages/staging"
	"github.com/mpyw/suve/internal/tui/styles"
)

// page is a full-screen view in the app shell's page stack. The active page is
// the top of the stack; the app forwards focus-relevant messages (keys the
// global map did not claim, mouse events over the page body, and window
// resizes) to it. Real pages (browser, diff, staging) land in later steps; the
// skeleton ships a placeholder.
type page interface {
	// Update handles a forwarded message and returns the (possibly replaced)
	// page plus any command. Returning a different page lets a page hand off to
	// another without the app knowing the concrete types.
	Update(tea.Msg) (page, tea.Cmd)
	// View renders the page body into the given content area.
	View(width, height int) string
	// capturesInput reports whether the page currently has a text input focused
	// that must receive raw keystrokes. While it does, the app suppresses its
	// global key map (reserving only ctrl+c) and forwards keys straight to the
	// page, so typing q/1/2/3/y/?/tab into a filter never quits or switches tabs.
	capturesInput() bool
}

// placeholderPage is the Step 2 stand-in for a real page: it centers a muted
// notice naming the tab and the step its page arrives in. It holds no state, so
// Update is a no-op.
type placeholderPage struct {
	tab    string
	notice string
	styles styles.Styles
}

// newPlaceholderPage builds the placeholder for a tab service key.
func newPlaceholderPage(st styles.Styles, tab, notice string) placeholderPage {
	return placeholderPage{tab: tab, notice: notice, styles: st}
}

// Update is a no-op: the placeholder has nothing to react to.
func (p placeholderPage) Update(tea.Msg) (page, tea.Cmd) {
	return p, nil
}

// capturesInput is always false: the placeholder has no text input.
func (p placeholderPage) capturesInput() bool { return false }

// View centers the placeholder notice in the content area.
func (p placeholderPage) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	body := p.styles.PageHint.Render(p.notice)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
}

// copyable is implemented by a page that supplies the `y`-copy value. Copying
// never changes the pane's mask state — a masked secret is copied to the
// clipboard but stays masked on screen (#689). The app consults the active page
// for it.
type copyable interface {
	CopyText() (string, bool)
}

// browserPage adapts *browser.Model to the app's page interface (whose Update
// returns the page interface, not the concrete type). It also forwards Init and
// the copy seam. The wrapped pointer means widget mutations persist.
type browserPage struct{ m *browser.Model }

func (p browserPage) Update(msg tea.Msg) (page, tea.Cmd) {
	m, cmd := p.m.Update(msg)

	return browserPage{m: m}, cmd
}

func (p browserPage) View(width, height int) string { return p.m.View(width, height) }
func (p browserPage) Init() tea.Cmd                 { return p.m.Init() }
func (p browserPage) CopyText() (string, bool)      { return p.m.CopyText() }
func (p browserPage) capturesInput() bool           { return p.m.CapturesInput() }
func (p browserPage) HelpKeyMap() help.KeyMap       { return p.m.HelpKeyMap() }

// diffPage adapts *diff.Model to the app's page interface.
type diffPage struct{ m *diff.Model }

func (p diffPage) Update(msg tea.Msg) (page, tea.Cmd) {
	m, cmd := p.m.Update(msg)

	return diffPage{m: m}, cmd
}

func (p diffPage) View(width, height int) string { return p.m.View(width, height) }
func (p diffPage) Init() tea.Cmd                 { return p.m.Init() }
func (p diffPage) HelpKeyMap() help.KeyMap       { return p.m.HelpKeyMap() }

// capturesInput is always false: the diff page has no text input (its keys are
// scroll/parse-json/back, all safe to route through the global map).
func (p diffPage) capturesInput() bool { return false }

// stagingPage adapts *staging.Model to the app's page interface.
type stagingPage struct{ m *staging.Model }

func (p stagingPage) Update(msg tea.Msg) (page, tea.Cmd) {
	m, cmd := p.m.Update(msg)

	return stagingPage{m: m}, cmd
}

func (p stagingPage) View(width, height int) string { return p.m.View(width, height) }
func (p stagingPage) Init() tea.Cmd                 { return p.m.Init() }
func (p stagingPage) HelpKeyMap() help.KeyMap       { return p.m.HelpKeyMap() }

// capturesInput is always false: the staging page has no text input.
func (p stagingPage) capturesInput() bool { return false }

// newStagingPage builds the staging page adapter over the offered services'
// staging seams. token is the page-generation identity the app bumps per page
// creation so a superseded prior page's in-flight result is dropped (#1011).
func newStagingPage(
	ctx context.Context, token int, services []data.StagingService, st styles.Styles, km keys.Map,
) stagingPage {
	return stagingPage{m: staging.New(ctx, token, services, st, km)}
}

// newStaticDiffPage builds a diff page over already-known content (the staging
// page's remote-vs-staged detail).
func newStaticDiffPage(content data.DiffContent, st styles.Styles, km keys.Map) diffPage {
	return diffPage{m: diff.NewStatic(content, st, km)}
}

// newBrowserPage builds the browser page adapter for a service source. token is
// the page-generation identity the app bumps per page creation so a superseded
// prior page's in-flight response is dropped rather than spliced in (#746).
// namespace is the launch App Configuration namespace the browser's namespace
// filter starts on.
func newBrowserPage(
	ctx context.Context, token int, source data.Source, staging data.StagingProbe, namespace string,
	st styles.Styles, km keys.Map,
) browserPage {
	return browserPage{m: browser.New(ctx, token, source, staging, namespace, st, km)}
}

// newDiffPage builds the diff page adapter from a navigation request.
func newDiffPage(ctx context.Context, req nav.OpenDiff, st styles.Styles, km keys.Map) diffPage {
	return diffPage{m: diff.New(ctx, req, st, km)}
}

// initPage returns a page's Init command when it has one (real pages), or nil
// (the placeholder).
func initPage(p page) tea.Cmd {
	if ip, ok := p.(interface{ Init() tea.Cmd }); ok {
		return ip.Init()
	}

	return nil
}
