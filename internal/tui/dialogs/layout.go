package dialogs

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// dialogChrome is the column overhead the shell's dialog frame adds around the
// content (a 1-cell rounded border plus 2 cells of horizontal padding on each
// side; see styles.Dialog Padding(0,2), #698); a dialog caps its lines at
// width−dialogChrome so a long name, error, or result line wraps inside the box
// instead of clipping its border. It MUST track the Dialog style's horizontal
// frame size — the shell un-offsets a modal click by that same frame.
const dialogChrome = 6

// dialogFrameHeight is the row overhead the shell's dialog frame adds above and
// below the content (the rounded border's top and bottom rows). Dialog padding is
// horizontal-only (Padding(0,2), #698) so the row budget is unchanged and the
// tallest fixed dialog (delete confirm) still fits the 60×16 minimum. A dialog
// caps a scrollable body at height−dialogFrameHeight (less its own pinned rows)
// so the whole box stays on-screen at the minimum supported size.
const dialogFrameHeight = 2

// minDialogContent floors the wrap width so a very narrow terminal still wraps
// to something legible rather than one column.
const minDialogContent = 24

// dialogLayout tracks the terminal size the shell fans to a dialog (via the
// seeded/forwarded WindowSizeMsg) and derives the width dialog content wraps to
// and the inner height a scrollable dialog may use. Every dialog embeds it so
// size-awareness is uniform: long lines wrap to contentWidth, and a body taller
// than the screen is capped/scrolled rather than clipped off the bottom edge.
type dialogLayout struct {
	// width / height are the terminal size (from the last WindowSizeMsg). Zero
	// until the first size arrives, which every helper treats as "not yet sized".
	width  int
	height int
}

// setSize records the terminal size a WindowSizeMsg carries.
func (l *dialogLayout) setSize(msg tea.WindowSizeMsg) {
	l.width, l.height = msg.Width, msg.Height
}

// sized reports whether a WindowSizeMsg has arrived. A dialog renders uncapped
// (natural size) until it knows the terminal size, so a size-less unit render
// still shows every line.
func (l *dialogLayout) sized() bool { return l.width > 0 && l.height > 0 }

// contentWidth is the inner width dialog content may fill: the terminal width
// less the shell's dialog frame, floored so a narrow terminal still wraps. Zero
// (before the first WindowSizeMsg) means "don't wrap".
func (l *dialogLayout) contentWidth() int {
	if l.width <= 0 {
		return 0
	}

	return max(l.width-dialogChrome, minDialogContent)
}

// availHeight is the number of body rows a dialog may draw inside the shell's
// frame: the terminal height less the border. Zero (unsized) means "no cap".
func (l *dialogLayout) availHeight() int {
	if l.height <= 0 {
		return 0
	}

	return max(l.height-dialogFrameHeight, 1)
}

// fit wraps one already-styled line to the content width when it would overflow,
// so a long title, name, error, or result line stays inside the box instead of
// pushing the border off-screen. Short lines pass through untouched so the box
// keeps its natural width when nothing wraps.
func (l *dialogLayout) fit(line string) string {
	if w := l.contentWidth(); w > 0 && lipgloss.Width(line) > w {
		return lipgloss.NewStyle().Width(w).Render(line)
	}

	return line
}

// wrapCapped wraps a line to the content width and caps it to maxRows rows
// (maxRows ≤ 0 means no cap). A dialog that cannot scroll its body — the delete
// confirm and the huh-form footers — renders a long provider error through it so
// the error stays inside the box and the essential controls and close hint stay
// on-screen, instead of the error pushing them off the bottom. When unsized
// (contentWidth 0) it returns the line unchanged so a size-less render is whole.
func (l *dialogLayout) wrapCapped(line string, maxRows int) string {
	w := l.contentWidth()
	if w <= 0 {
		return line
	}

	st := lipgloss.NewStyle().Width(w)
	if maxRows > 0 {
		st = st.MaxHeight(maxRows)
	}

	return st.Render(line)
}

// errBudget is the maximum rows an inline error may occupy so the whole dialog
// still fits: the frame's inner height less the fixed rows around the error
// (title, spacers, controls/body, hint) and any reserved body. Floored at 1 so
// an error always shows at least a line; 0 (unsized) means "no cap".
func (l *dialogLayout) errBudget(fixedRows int) int {
	if !l.sized() {
		return 0
	}

	return max(l.availHeight()-fixedRows, 1)
}
