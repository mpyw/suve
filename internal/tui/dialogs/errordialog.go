package dialogs

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/hit"
	"github.com/mpyw/suve/internal/tui/styles"
)

// errorSpacerRows is the two blank lines the error dialog pins (one below the
// title, one above the close hint) around the scrollable message body.
const errorSpacerRows = 2

// regionClose is the close-hint region ID (shared by the error dialog and any
// confirm dialog whose close hint is clickable).
const regionClose = "close"

// errorDialog is a plain modal that surfaces a message the app could not handle
// inline — a blocked operation (e.g. creating while viewing all namespaces) or a
// staging key-loss hard-fail. It never mutates anything; Enter or Esc dismisses
// it (the app owns Esc; Enter emits CanceledMsg). Long messages wrap to the
// dialog width and scroll inside a viewport so neither the text nor the close
// hint clips off-screen at the minimum supported size.
type errorDialog struct {
	dialogLayout

	styles  styles.Styles
	title   string
	message string
	// vp scrolls the message body when it is taller than the box can show; the
	// title and close hint are rendered outside it so they stay pinned.
	vp viewport.Model
	// scrollable records whether the last synced body overflowed the viewport, so
	// the close hint advertises the scroll keys only when they do something.
	scrollable bool
	// hits maps a click on the close hint to the same dismissal enter/esc perform.
	hits *hit.Map
}

// NewError builds an error dialog with a title and message.
func NewError(st styles.Styles, title, message string) Model {
	if title == "" {
		title = "Error"
	}

	return &errorDialog{styles: st, title: title, message: message, vp: viewport.New()}
}

func (d *errorDialog) Busy() bool { return false }

func (d *errorDialog) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.setSize(msg)
		d.syncViewport()

		return d, nil
	case tea.MouseWheelMsg:
		return d, scrollViewport(&d.vp, msg)
	case tea.MouseClickMsg:
		if id, _, _, ok := d.hits.At(msg.X, msg.Y); ok && id == regionClose {
			return d, canceledCmd
		}

		return d, nil
	case tea.KeyPressMsg:
		if key.Matches(msg, navSelect) {
			return d, canceledCmd
		}

		// Any other key scrolls the message body (↑↓/j/k, pgup/pgdn, etc.); Esc is
		// intercepted by the shell before it reaches here, so it still closes.
		return d, scrollViewport(&d.vp, msg)
	}

	return d, nil
}

// syncViewport (re)builds the wrapped message body and sizes the viewport to
// min(needed, height−errorChrome), so a long message scrolls inside the box
// while the title and close hint stay pinned. It is a no-op until a
// WindowSizeMsg arrives (before that the message renders inline, uncapped).
func (d *errorDialog) syncViewport() {
	if !d.sized() {
		return
	}

	body := d.body()
	lines := max(lipgloss.Height(body), 1)
	// Reserve the taller (scroll) hint variant so the budget is correct regardless
	// of whether the body ends up scrollable — d.scrollable is only set below.
	around := lipgloss.Height(d.header()) + lipgloss.Height(hintText(true)) + errorSpacerRows
	avail := max(d.availHeight()-around, 1)
	height := min(lines, avail)

	d.scrollable = lines > height
	// Size the viewport to the body's natural width (capped at the content width),
	// so a short message keeps a snug box while a wrapped long one fills the width.
	d.vp.SetWidth(min(lipgloss.Width(body), d.contentWidth()))
	d.vp.SetHeight(height)
	d.vp.SetContent(body)
}

func (d *errorDialog) View() string {
	var b strings.Builder

	header := d.header()
	b.WriteString(header)
	b.WriteString("\n\n")

	// Once sized, the message scrolls inside the viewport; before the first
	// WindowSizeMsg it renders inline (uncapped) so a size-less unit render still
	// shows the whole message.
	body := d.message
	if d.sized() {
		body = d.vp.View()
	}

	b.WriteString(body)
	b.WriteString("\n\n")

	hint := d.hint()
	b.WriteString(hint)

	// The close hint is clickable (the shell forwards a content-local click here),
	// reducing to the same dismissal enter/esc perform.
	hintY := lipgloss.Height(header) + 1 + lipgloss.Height(body) + 1
	d.hits = hit.New(hit.Region(regionClose, 0, hintY, lipgloss.Width(hint), 1))

	return b.String()
}

// header renders the error title, wrapped to the dialog width so a long title
// does not overflow the box.
func (d *errorDialog) header() string {
	return d.fit(d.styles.ErrorText.Render(d.title))
}

// body renders the message wrapped to the content width so a long provider error
// or key-loss message stays inside the box instead of clipping at the edge.
func (d *errorDialog) body() string {
	return d.fit(d.message)
}

// hint pins the close hint, advertising the scroll keys only when the message
// actually overflows the viewport.
func (d *errorDialog) hint() string {
	return d.styles.PageHint.Render(hintText(d.scrollable))
}

// hintText is the close-hint text, with the scroll keys when the body scrolls.
func hintText(scrollable bool) string {
	if scrollable {
		return "↑↓/pgup/pgdn: scroll · enter/esc: close"
	}

	return "enter/esc: close"
}
