//nolint:testpackage // white-box: exercises the unexported modeConfirm component
package dialogs

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/tui/styles"
)

// TestModeConfirm_Keys pins the shared Stage/Apply popup's key contract: ←/↑ pick
// Stage, →/↓ pick Apply immediately, enter commits, esc goes back, and any other
// key is swallowed (so it never leaks into the form underneath).
func TestModeConfirm_Keys(t *testing.T) {
	t.Parallel()

	c := newModeConfirm("New parameter", true)

	// Right/Down pick Apply immediately (staged = false).
	assert.Equal(t, confirmNone, c.Update(keyRight()))
	assert.False(t, c.staged, "→ picks Apply immediately")
	assert.Equal(t, confirmNone, c.Update(keyLeft()))
	assert.True(t, c.staged, "← picks Stage")
	assert.Equal(t, confirmNone, c.Update(tea.KeyPressMsg{Code: tea.KeyDown}))
	assert.False(t, c.staged, "↓ picks Apply immediately")
	assert.Equal(t, confirmNone, c.Update(tea.KeyPressMsg{Code: tea.KeyUp}))
	assert.True(t, c.staged, "↑ picks Stage")

	// Enter commits, esc goes back, an unrelated key is swallowed as confirmNone.
	assert.Equal(t, confirmExecute, c.Update(keyEnter()))
	assert.Equal(t, confirmBack, c.Update(keyEsc()))
	assert.Equal(t, confirmNone, c.Update(keyMsg('x')))
}

// TestModeConfirm_View pins that the popup renders its title, the Stage/Apply radio
// reflecting the current selection, and the key hint.
func TestModeConfirm_View(t *testing.T) {
	t.Parallel()

	st := styles.New()

	stagedConfirm := newModeConfirm("Delete secret", true)
	staged := stagedConfirm.View(st)
	assert.Contains(t, staged, "Delete secret")
	assert.Contains(t, staged, "(•) Stage")
	assert.Contains(t, staged, "Apply immediately")
	assert.Contains(t, staged, "enter: confirm")

	immediateConfirm := newModeConfirm("Delete secret", false)
	immediate := immediateConfirm.View(st)
	assert.Contains(t, immediate, "(•) Apply immediately", "an immediate selection marks the Apply radio")
}
