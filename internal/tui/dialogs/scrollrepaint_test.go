//nolint:testpackage // white-box: exercises the unexported repaint helpers
package dialogs

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"github.com/stretchr/testify/assert"
)

//nolint:gochecknoglobals // test-only type sentinel
var clearScreenType = reflect.TypeOf(tea.ClearScreen())

func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if msg == nil {
		return nil
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drain(c)...)
		}

		return out
	}

	return []tea.Msg{msg}
}

func hasClearScreen(cmd tea.Cmd) bool {
	for _, m := range drain(cmd) {
		if reflect.TypeOf(m) == clearScreenType {
			return true
		}
	}

	return false
}

// TestScrollViewportGatesOnOffsetChange pins that a dialog viewport repaint fires
// only when the offset actually changes on an affected terminal.
func TestScrollViewportGatesOnOffsetChange(t *testing.T) {
	t.Setenv("CLOUD_SHELL", "")
	t.Setenv("AZUREPS_HOST_ENVIRONMENT", "")
	t.Setenv("SUVE_TUI_FULL_REPAINT", "")
	t.Setenv("AWS_EXECUTION_ENV", "CloudShell")

	vp := viewport.New()
	vp.SetWidth(20)
	vp.SetHeight(3)
	vp.SetContent("l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9")

	// Already at the top: scrolling up does not move the offset -> no repaint.
	assert.Equal(t, 0, vp.YOffset())
	assert.False(t, hasClearScreen(scrollViewport(&vp, tea.KeyPressMsg{Code: tea.KeyPgUp})),
		"a no-op scroll at the top must not repaint")

	// Page-down from the top changes the offset -> repaint.
	assert.True(t, hasClearScreen(scrollViewport(&vp, tea.KeyPressMsg{Code: tea.KeyPgDown})),
		"scrolling from the top must force a repaint in CloudShell")
	assert.Positive(t, vp.YOffset(), "sanity: the viewport actually scrolled")
}

// TestFormKeyMayScroll pins the field-type-aware decision: a single-line Input
// scrolls only when focus moves (so typing never repaints), while a multi-line
// Text may scroll on any key.
func TestFormKeyMayScroll(t *testing.T) {
	t.Parallel()

	inputForm := huh.NewForm(huh.NewGroup(huh.NewInput().Key("name")))
	inputForm.Init()

	textForm := huh.NewForm(huh.NewGroup(huh.NewText().Key("body")))
	textForm.Init()

	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	typing := tea.KeyPressMsg{Code: 'a', Text: "a"}

	assert.True(t, formKeyMayScroll(inputForm, tab), "Input: a focus-move key can scroll")
	assert.False(t, formKeyMayScroll(inputForm, typing), "Input: typing cannot scroll")
	assert.True(t, formKeyMayScroll(textForm, tab), "Text: focus-move key can scroll")
	assert.True(t, formKeyMayScroll(textForm, typing), "Text: typing can wrap-scroll")
}

// TestRepaintFormScroll pins the gating + ordering: on an affected terminal a
// scroll-capable key forces a repaint (via tea.Sequence, so it lands after the
// async focus move), while typing in an Input, a non-key message, or a native
// terminal do not.
func TestRepaintFormScroll(t *testing.T) {
	t.Setenv("CLOUD_SHELL", "")
	t.Setenv("AZUREPS_HOST_ENVIRONMENT", "")
	t.Setenv("SUVE_TUI_FULL_REPAINT", "")

	form := huh.NewForm(huh.NewGroup(huh.NewInput().Key("name")))
	form.Init()

	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	typing := tea.KeyPressMsg{Code: 'a', Text: "a"}

	// Native terminal: never repaints, even on a focus-move key.
	t.Setenv("AWS_EXECUTION_ENV", "")
	assert.False(t, hasClearScreen(repaintFormScroll(form, tab, nil)), "native terminal: no repaint")

	// CloudShell: focus-move key on an Input repaints; typing does not.
	t.Setenv("AWS_EXECUTION_ENV", "CloudShell")
	assert.True(t, hasClearScreen(repaintFormScroll(form, tab, nil)), "CloudShell: focus-move key repaints")
	assert.False(t, hasClearScreen(repaintFormScroll(form, typing, nil)), "CloudShell: Input typing does not repaint")
	assert.False(t, hasClearScreen(repaintFormScroll(form, tea.WindowSizeMsg{Width: 1, Height: 1}, nil)),
		"CloudShell: a non-key message does not repaint")
}
