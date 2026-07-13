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
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
)

// Service keys shared across the dialogs (the service axis).
const (
	serviceParam  = "param"
	serviceSecret = "secret"
)

// discardNotice is the inline warning a free-text form shows after the first Esc
// on a dirty form (#790); a second consecutive Esc then discards.
const discardNotice = "unsaved changes — press esc again to discard"

// submitKey submits a create/edit/tag form from any field (#791): ctrl+s is the
// portable fallback that works in every terminal, while shift+enter is the
// enhanced-keyboard binding (indistinguishable from a plain Enter unless the
// terminal's Kitty keyboard protocol is active). It is handled by the dialog
// itself — not the embedded huh form, which can only submit from its last field —
// so a multi-line Value textarea (where Enter now inserts a newline) can still be
// submitted while focused.
//
//nolint:gochecknoglobals // immutable dialog-local binding
var submitKey = key.NewBinding(key.WithKeys("ctrl+s", "shift+enter"))

// escKey is the discard/cancel binding the shell forwards into a discard-guarded
// form (see EscInterceptor) so the form itself decides whether to arm a discard
// confirmation (dirty) or close immediately (clean).
//
//nolint:gochecknoglobals // immutable dialog-local binding
var escKey = key.NewBinding(key.WithKeys("esc"))

// EscInterceptor is an optional dialog capability. A free-text form that guards
// against discarding unsaved input implements it so the shell forwards the Back
// (Esc) key into the dialog's Update — where a dirty form arms a discard
// confirmation (a second Esc then discards) — instead of bare-popping it. A
// clean form returns CanceledMsg on the first Esc, so the shell closes it exactly
// as before. The shell still suppresses Esc entirely while the dialog is Busy (a
// mutation in flight is never interrupted).
type EscInterceptor interface {
	InterceptEsc() bool
}

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

// DismissReloader is an optional dialog capability. A dialog that has already
// mutated by the time it can be dismissed — the apply results view — returns a
// non-nil command from DismissCmd so that closing it with Back (Esc) runs the
// same pop+reload+voice as its confirm key, instead of the shell's bare pop
// (which would leave the staging page rendering just-applied items as still
// staged). Returning nil means "fall back to a bare dismiss".
type DismissReloader interface {
	DismissCmd() tea.Cmd
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
