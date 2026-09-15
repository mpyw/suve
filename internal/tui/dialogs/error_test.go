//nolint:testpackage // white-box: drives the error dialog's Update/View and close hit region
package dialogs

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/styles"
)

// TestErrorDialog_MouseClickCloses pins that clicking the error dialog's close
// hint dismisses it, reducing to the same CanceledMsg enter/esc emit.
func TestErrorDialog_MouseClickCloses(t *testing.T) {
	t.Parallel()

	m := NewError(styles.New(), "Cannot create here", "Select a single namespace before creating.")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_ = m.View()

	d, ok := m.(*errorDialog)
	require.True(t, ok)

	_, cmd := d.Update(clickAt(t, d.hits, regionClose))
	require.NotNil(t, cmd, "clicking the close hint dispatches")
	_, isCancel := cmd().(CanceledMsg)
	assert.True(t, isCancel, "clicking close dismisses (like enter/esc)")
}

// TestErrorDialog_Update pins the error dialog's message handling: enter/esc
// dismisses (CanceledMsg); a scroll key routes into the viewport when the body
// overflows; and a resize (re)builds the scrollable body.
func TestErrorDialog_Update(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a very long provider error line that must wrap and then scroll inside the box\n", 200)

	m := NewError(styles.New(), "Cannot proceed", long)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})

	d, ok := m.(*errorDialog)
	require.True(t, ok)
	assert.True(t, d.scrollable, "a body taller than the box scrolls")

	// A scroll key (pgdn) advances the viewport off the top (not a dismissal).
	require.True(t, d.vp.AtTop(), "the message opens at the top")
	_, cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.Nil(t, cmd, "a scroll key does not dismiss")
	assert.False(t, d.vp.AtTop(), "a scroll key advances the viewport")

	// Enter dismisses with a CanceledMsg.
	_, cmd = d.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter dispatches")
	_, isCancel := cmd().(CanceledMsg)
	assert.True(t, isCancel, "enter dismisses the error dialog")

	// The mouse wheel scrolls the body too.
	_, _ = d.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
}

// TestErrorDialog_TitleDefaults pins that an empty title falls back to "Error"
// and Busy() is always false (the error dialog never mutates).
func TestErrorDialog_TitleDefaults(t *testing.T) {
	t.Parallel()

	m := NewError(styles.New(), "", "something went wrong")
	d, ok := m.(*errorDialog)
	require.True(t, ok)

	assert.Equal(t, "Error", d.title, "an empty title falls back to \"Error\"")
	assert.False(t, d.Busy(), "the error dialog is never busy")
	assert.Contains(t, d.View(), "something went wrong", "a size-less render shows the whole message inline")
}

// TestErrorDialog_LongMessageWrapsAndScrollsMinSize pins that a long
// provider/key-loss error wraps to the dialog width and scrolls inside a
// viewport, so neither the message nor the close hint clips off-screen at the
// minimum size; a short message neither scrolls nor advertises scroll keys.
func TestErrorDialog_LongMessageWrapsAndScrollsMinSize(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("the staging data key could not be recovered from the keychain. ", 10)

	m := NewError(styles.New(), "Staging key lost", long)
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	d, ok := m.(*errorDialog)
	require.True(t, ok)
	assert.True(t, d.scrollable, "a message taller than the box scrolls")

	view := m.View()
	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"the wrapped message never overflows the dialog width")
	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"the whole dialog fits within the terminal height")
	assert.Contains(t, stripANSI(view), "scroll", "the hint advertises scroll when the body overflows")
	assert.Contains(t, stripANSI(view), "enter/esc: close", "the close hint stays pinned")

	short := NewError(styles.New(), "Blocked", "Pick one namespace first.")
	short, _ = short.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})
	sd, ok := short.(*errorDialog)
	require.True(t, ok)
	assert.False(t, sd.scrollable, "a short message does not scroll")
	assert.NotContains(t, stripANSI(short.View()), "scroll", "no scroll keys advertised when it fits")
}
