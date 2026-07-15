package dialogs

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/termquirk"
)

// scrollViewport forwards msg to a dialog's scrollable viewport and, when its
// scroll offset actually changes on a terminal that mishandles Bubble Tea's
// scroll-region optimization (CloudShell), forces a full repaint so the scroll
// renders cleanly (see internal/tui/termquirk).
//
// The viewport scrolls synchronously inside Update, so the offset delta is known
// immediately and the ClearScreen batch renders the already-scrolled model. huh
// forms are deliberately NOT handled here: they defer focus movement through an
// async NextField/PrevField command, so a batched ClearScreen would race the
// scroll — that is tracked as a separate follow-up.
func scrollViewport(vp *viewport.Model, msg tea.Msg) tea.Cmd {
	before := vp.YOffset()

	var cmd tea.Cmd

	*vp, cmd = vp.Update(msg)

	return termquirk.RepaintOnScroll(vp.YOffset() != before, cmd)
}
