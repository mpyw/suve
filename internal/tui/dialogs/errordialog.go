package dialogs

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/styles"
)

// errorDialog is a plain modal that surfaces a message the app could not handle
// inline — a blocked operation (e.g. creating while viewing all namespaces) or a
// staging key-loss hard-fail. It never mutates anything; Enter or Esc dismisses
// it (the app owns Esc; Enter emits CanceledMsg).
type errorDialog struct {
	styles  styles.Styles
	title   string
	message string
}

// NewError builds an error dialog with a title and message.
func NewError(st styles.Styles, title, message string) Model {
	if title == "" {
		title = "Error"
	}

	return &errorDialog{styles: st, title: title, message: message}
}

func (d *errorDialog) Busy() bool { return false }

func (d *errorDialog) Update(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && key.Matches(k, navSelect) {
		return d, canceledCmd
	}

	return d, nil
}

func (d *errorDialog) View() string {
	var b strings.Builder

	b.WriteString(d.styles.ErrorText.Render(d.title))
	b.WriteString("\n\n")
	b.WriteString(d.message)
	b.WriteString("\n\n")
	b.WriteString(d.styles.PageHint.Render("enter/esc: close"))

	return b.String()
}
