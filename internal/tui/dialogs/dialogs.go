// Package dialogs implements the TUI's modal mutation dialogs: the create/edit
// entry form (a charm.land/huh/v2 form embedded as a model, with a $EDITOR
// handoff), the delete confirm, the tag add/remove form, the restore form, and
// a plain error dialog. Every mutation dialog carries a staged-by-default /
// apply-immediately mode toggle (hidden when the service has no staging, so the
// write is always immediate), and routes through the provider-neutral
// data.Mutator seam — staged writes to internal/usecase/staging, immediate
// writes to the direct param/secret use cases. The app shell owns the dialog
// stack and dismissal; a dialog reports Busy() so the shell suppresses dismissal
// while an operation is in flight.
package dialogs

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
)

// Service keys shared across the dialogs (the service axis).
const (
	serviceParam  = "param"
	serviceSecret = "secret"
)

// Model is a modal dialog embedded in the app shell's dialog stack. It mirrors
// the app's page contract but returns its own concrete interface (the app adapts
// it). While Busy() reports true the shell must not dismiss it (a mutation is in
// flight — GUI "Modal busy" parity).
type Model interface {
	// Update handles a forwarded message and returns the (possibly replaced)
	// dialog plus any command.
	Update(tea.Msg) (Model, tea.Cmd)
	// View renders the dialog's inner content (the shell frames and centers it).
	View() string
	// Busy reports whether an operation is mid-flight, so the shell suppresses
	// dismissal and the dialog swallows further input (double-submit guard).
	Busy() bool
}

// MutationDoneMsg is emitted when a mutation succeeds. The app pops the dialog,
// reloads the affected service's browser (list/detail/staged badges), refreshes
// the staging tab count, and voices Status.
type MutationDoneMsg struct {
	// Service is the affected service ("param"/"secret"), so the app reloads the
	// right browser page.
	Service string
	// Status is the one-line outcome to voice (staged/applied/skipped/unstaged).
	Status string
	// Staged reports whether the write was staged (vs applied immediately).
	Staged bool
}

// CanceledMsg is emitted when a dialog is dismissed without an action (the
// Cancel button, or a huh form abort). The app pops the dialog.
type CanceledMsg struct{}

// mutationResultMsg carries a mutation's result back into the dialog's Update.
type mutationResultMsg struct {
	outcome data.WriteOutcome
	err     error
}

// runMutation runs fn off the update loop and reports the result.
func runMutation(fn func() (data.WriteOutcome, error)) tea.Cmd {
	return func() tea.Msg {
		out, err := fn()

		return mutationResultMsg{outcome: out, err: err}
	}
}

// canceledCmd emits CanceledMsg.
func canceledCmd() tea.Msg { return CanceledMsg{} }

// doneCmd emits MutationDoneMsg for a completed mutation.
func doneCmd(service, status string, staged bool) tea.Cmd {
	return func() tea.Msg {
		return MutationDoneMsg{Service: service, Status: status, Staged: staged}
	}
}
