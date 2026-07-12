package tui

import (
	"context"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/pages/browser"
	"github.com/mpyw/suve/internal/tui/pages/diff"
	"github.com/mpyw/suve/internal/tui/pages/staging"
	"github.com/mpyw/suve/internal/tui/styles"
)

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
// staging seams.
func newStagingPage(ctx context.Context, services []data.StagingService, st styles.Styles, km keys.Map) stagingPage {
	return stagingPage{m: staging.New(ctx, services, st, km)}
}

// newStaticDiffPage builds a diff page over already-known content (the staging
// page's remote-vs-staged detail).
func newStaticDiffPage(content data.DiffContent, st styles.Styles, km keys.Map) diffPage {
	return diffPage{m: diff.NewStatic(content, st, km)}
}

// newBrowserPage builds the browser page adapter for a service source.
func newBrowserPage(
	ctx context.Context, source data.Source, staging data.StagingProbe, st styles.Styles, km keys.Map,
) browserPage {
	return browserPage{m: browser.New(ctx, source, staging, st, km)}
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
