package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/dialogs"
)

// dialogAdapter adapts a dialogs.Model (whose Update returns dialogs.Model) to
// the app's dialog interface (whose Update returns the app's unexported dialog
// type), mirroring how pages_wire.go adapts the pages. The wrapped value carries
// its state forward because dialogs.Model implementations are pointers.
type dialogAdapter struct{ m dialogs.Model }

func (a dialogAdapter) Update(msg tea.Msg) (dialog, tea.Cmd) {
	next, cmd := a.m.Update(msg)

	return dialogAdapter{m: next}, cmd
}

func (a dialogAdapter) View() string { return a.m.View() }
func (a dialogAdapter) busy() bool   { return a.m.Busy() }

// DismissCmd forwards the optional dialogs.DismissReloader seam through the
// adapter, mirroring how pages_wire.go's copyable seam forwards CopyText. The
// app stores every dialog as a dialogAdapter, so the shell's Back handler
// asserts DismissReloader against the adapter — not the wrapped dialog. Without
// this the assertion could never succeed and Esc on the apply-results view
// would bare-pop, skipping the post-apply reload (#744). Returns the wrapped
// dialog's command when it implements DismissReloader, else nil so the shell
// bare-dismisses exactly as before.
func (a dialogAdapter) DismissCmd() tea.Cmd {
	if d, ok := a.m.(dialogs.DismissReloader); ok {
		return d.DismissCmd()
	}

	return nil
}
