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
