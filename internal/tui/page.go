package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
