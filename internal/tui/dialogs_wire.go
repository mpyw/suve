//declscope:namespace app
//
// dialogAdapter wraps every dialog the App pushes, and the App's dialog
// stack (push, pop, update) sits beside it; the two are one mechanism.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/dialogs"
)

// dialogAdapter adapts a dialogs.Model (whose Update returns dialogs.Model) to
// the app's dialog interface (whose Update returns the app's unexported dialog
// type), mirroring how page.go adapts the pages. The wrapped value carries
// its state forward because dialogs.Model implementations are pointers.
type dialogAdapter struct{ m dialogs.Model }

func (a dialogAdapter) Update(msg tea.Msg) (dialog, tea.Cmd) {
	next, cmd := a.m.Update(msg)

	return dialogAdapter{m: next}, cmd
}

func (a dialogAdapter) View() string { return a.m.View() }
func (a dialogAdapter) busy() bool   { return a.m.Busy() }

// DismissCmd forwards the optional dialogs.DismissReloader seam through the
// adapter, mirroring how page.go's copyable seam forwards CopyText. The
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

// interceptEsc forwards the optional dialogs.EscInterceptor seam through the
// adapter (mirroring DismissCmd): the app stores every dialog as a dialogAdapter,
// so the shell's Back handler asserts the seam against the adapter, not the
// wrapped dialog. Returns the wrapped dialog's decision when it implements
// EscInterceptor (the create/edit/tag forms' discard guard, #790), else false so
// the shell dismisses exactly as before.
func (a dialogAdapter) interceptEsc() bool {
	if d, ok := a.m.(dialogs.EscInterceptor); ok {
		return d.InterceptEsc()
	}

	return false
}

// pushDialog appends a dialog to the modal stack, seeds it with the current size
// (so its embedded form lays out before the first render), clears any transient
// status, and returns the dialog's Init command.
//
// Dialog-open requests arrive as async commands (a page emits
// func() tea.Msg { return nav.Open*{...} }), so a rapid double-press of a
// dialog-open key (e/n/d/t/a) can emit two Open* commands before the first
// dialog lands, and both would otherwise push an identical dialog — Esc then
// reveals the duplicate underneath. Guard against that here, the single choke
// point every dialog push flows through: a dialog is modal (once one is on the
// stack it captures all input, so the page can no longer emit Open*), so any
// Open* arriving while a dialog is already open is a stale in-flight duplicate.
// Drop it rather than stack a second dialog.
func (m *App) pushDialog(d dialogs.Model, initCmd tea.Cmd) tea.Cmd {
	if len(m.dialogs) > 0 {
		return nil
	}

	m.status = ""
	m.dialogs = append(m.dialogs, dialogAdapter{m: d})

	if m.width > 0 && m.height > 0 {
		top := len(m.dialogs) - 1
		next, _ := m.dialogs[top].Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.dialogs[top] = next
	}

	return initCmd
}

// topDialogBusy reports whether the top dialog is mid-operation.
func (m *App) topDialogBusy() bool {
	if len(m.dialogs) == 0 {
		return false
	}

	return m.dialogs[len(m.dialogs)-1].busy()
}

// updateTopDialog forwards a message to the top dialog.
func (m *App) updateTopDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	top := len(m.dialogs) - 1
	d, cmd := m.dialogs[top].Update(msg)
	m.dialogs[top] = d

	return m, cmd
}

// popDialog removes the top dialog, if any.
func (m *App) popDialog() {
	if len(m.dialogs) > 0 {
		m.dialogs = m.dialogs[:len(m.dialogs)-1]
	}
}
