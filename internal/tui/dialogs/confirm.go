package dialogs

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/styles"
)

// confirmExecuteKey commits the Stage/Apply choice in the mode-confirm popup.
// Execution is Enter only: the popup is the final, deliberate step, so it does
// not also bind ctrl+s (which is the key that opened the popup from the form).
//
//nolint:gochecknoglobals // immutable dialog-local binding
var confirmExecuteKey = key.NewBinding(key.WithKeys("enter"))

// confirmResult is what a key press did to the mode-confirm popup.
type confirmResult int

const (
	// confirmNone: the key changed the selection or was ignored — stay open.
	confirmNone confirmResult = iota
	// confirmExecute: enter — run the mutation with the chosen mode.
	confirmExecute
	// confirmBack: esc — dismiss the popup and return to the form/controls.
	confirmBack
)

// modeConfirm is the shared Stage/Apply confirmation popup shown before a write
// commits. It replaces the old inline Mode toggle so the Stage-vs-Apply choice is
// always an explicit, visible step — never silently defaulted by a submit from a
// field that never reached the toggle. The owning dialog holds one, routes keys to
// Update while it is open, and renders View in place of its form/controls (the app
// shell frames and centers it, so it reads as a compact popup).
type modeConfirm struct {
	title  string
	staged bool
}

// newModeConfirm builds the popup seeded with the dialog's title and current mode
// (staged by default when the service supports staging).
func newModeConfirm(title string, staged bool) modeConfirm {
	return modeConfirm{title: title, staged: staged}
}

// Update folds a key press into the popup: ←/↑ pick Stage, →/↓ pick Apply
// immediately, enter commits, esc goes back. Any other key is ignored (the popup
// swallows it so it never leaks into the form underneath).
func (c *modeConfirm) Update(msg tea.KeyPressMsg) confirmResult {
	switch {
	case key.Matches(msg, escKey):
		return confirmBack
	case key.Matches(msg, confirmExecuteKey):
		return confirmExecute
	case key.Matches(msg, navLeft), key.Matches(msg, navUp), key.Matches(msg, navDec):
		c.staged = true
	case key.Matches(msg, navRight), key.Matches(msg, navDown), key.Matches(msg, navInc):
		c.staged = false
	}

	return confirmNone
}

// View renders the popup body: the owning dialog's title, the Stage/Apply radio
// (the same modeLabel the dialogs have always drawn), and the key hint.
func (c *modeConfirm) View(st styles.Styles) string {
	var b strings.Builder

	b.WriteString(st.PaneTitle.Render(c.title))
	b.WriteString("\n\n")
	b.WriteString(modeLabel(c.staged))
	b.WriteString("\n\n")
	b.WriteString(st.PageHint.Render("←→: choose · enter: confirm · esc: back"))

	return b.String()
}
