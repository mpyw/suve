//nolint:testpackage // white-box: exercises the unexported repaint helpers
package dialogs

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
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
