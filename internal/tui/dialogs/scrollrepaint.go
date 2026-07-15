package dialogs

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"

	"github.com/mpyw/suve/internal/tui/termquirk"
)

// scrollViewport forwards msg to a dialog's scrollable viewport and, when its
// scroll offset actually changes on a terminal that mishandles Bubble Tea's
// scroll-region optimization (CloudShell), forces a full repaint so the scroll
// renders cleanly (see internal/tui/termquirk).
//
// The viewport scrolls synchronously inside Update, so the offset delta is known
// immediately and the ClearScreen batch renders the already-scrolled model.
func scrollViewport(vp *viewport.Model, msg tea.Msg) tea.Cmd {
	before := vp.YOffset()

	var cmd tea.Cmd

	*vp, cmd = vp.Update(msg)

	return termquirk.RepaintOnScroll(vp.YOffset() != before, cmd)
}

// formFocusKeys are the keys that move focus between fields in a huh form (as
// opposed to editing text inside a single-line field). Only these can scroll a
// form whose focused field is a single-line Input.
//
//nolint:gochecknoglobals // immutable key set, mirrors the other dialog bindings
var formFocusKeys = key.NewBinding(key.WithKeys("tab", "shift+tab", "up", "down", "enter"))

// repaintFormScroll forces a full repaint after a key that may scroll a huh
// form's internal viewport, on a terminal that mishandles the scroll-region
// optimization (see termquirk); on native terminals it returns cmd untouched.
//
// Unlike the synchronous surfaces, huh moves focus through an async
// NextField/PrevField command, so the scroll happens in a later update. It
// therefore uses tea.Sequence (not tea.Batch): the messages are emitted in order
// and processed FIFO, so the ClearScreen lands AFTER the scroll has settled.
//
// To avoid repainting on every keystroke, a single-line Input (where typing
// cannot scroll) repaints only on focus-move keys; every other field kind — a
// multi-line Text that wraps, or a Select/MultiSelect navigated with j/k/g etc. —
// repaints on any key, since there the "typing vs. navigation" distinction does
// not hold.
func repaintFormScroll(form *huh.Form, msg tea.Msg, cmd tea.Cmd) tea.Cmd {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok || !termquirk.ScrollNeedsFullRepaint() || !formKeyMayScroll(form, kp) {
		return cmd
	}

	return tea.Sequence(cmd, tea.ClearScreen)
}

// formKeyMayScroll reports whether kp could scroll the form's internal viewport,
// given the focused field kind. A single-line Input scrolls only when focus
// moves off-screen (formFocusKeys); any other field may scroll on any key.
func formKeyMayScroll(form *huh.Form, kp tea.KeyPressMsg) bool {
	if _, isSingleLine := form.GetFocusedField().(*huh.Input); isSingleLine {
		return key.Matches(kp, formFocusKeys)
	}

	return true
}
