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
//
// This file is the package's core. The contract, messages and small widgets
// here are the shared vocabulary every dialog builds on, so the file carries no
// namespace prefix and its declarations are shared package-wide.
//
//declscope:core
//declscope:package
package dialogs

import (
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
)

// discardNotice is the inline warning a free-text form shows after the first Esc
// on a dirty form (#790); a second consecutive Esc then discards.
const discardNotice = "unsaved changes — press esc again to discard"

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

// NowFunc is the clock the delete dialog's "recoverable until" date is computed
// from. It is an exported package variable so a golden can pin the date
// deterministically.
//
//nolint:gochecknoglobals // swappable clock seam for the recoverable-until date
var NowFunc = time.Now

// stringError is a small sentinel error type for dialog validation.
type stringError string

func (e stringError) Error() string { return string(e) }

// Shared navigation bindings for the custom (non-huh) dialogs (delete/restore
// hint lines and toggles).
//
//nolint:gochecknoglobals // immutable dialog-local bindings
var (
	navUp     = key.NewBinding(key.WithKeys("up", "k"))
	navDown   = key.NewBinding(key.WithKeys("down", "j", "tab"))
	navLeft   = key.NewBinding(key.WithKeys("left", "h"))
	navRight  = key.NewBinding(key.WithKeys("right", "l"))
	navDec    = key.NewBinding(key.WithKeys("-", "_"))
	navInc    = key.NewBinding(key.WithKeys("+", "="))
	navSelect = key.NewBinding(key.WithKeys("enter", "space"))
)

// checkbox renders a "[x]"/"[ ]" checkbox.
func checkbox(on bool) string {
	if on {
		return "[x]"
	}

	return "[ ]"
}

// modeLabel renders the staged-vs-immediate mode as radio-style options.
func modeLabel(staged bool) string {
	if staged {
		return "(•) Stage    ( ) Apply immediately"
	}

	return "( ) Stage    (•) Apply immediately"
}

// clipName renders an entry name with its App Configuration namespace badge (a
// bare name for the null/default namespace and every other provider).
func clipName(name, namespace string) string {
	if namespace == "" {
		return name
	}

	return name + " @" + namespace
}

// requiredField builds a huh validator that rejects an empty/whitespace value.
func requiredField(label string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return stringError(label + " is required")
		}

		return nil
	}
}

// titleSpacerRows is the blank line the form dialogs draw between the title and
// the form body; it is reserved when budgeting the body's scrollable height.
const titleSpacerRows = 1

// minFormBody floors the embedded form's scrollable body height so a very short
// terminal still shows at least a field or two (the rest scrolls into view)
// rather than collapsing the form to nothing.
const minFormBody = 3

// buttonGap is the column gap between two side-by-side confirm buttons (the
// "    " separator), used to place the second button's hit region.
const buttonGap = 4

// regionClose is the close-hint region ID (shared by the error dialog and any
// confirm dialog whose close hint is clickable).
const regionClose = "close"

// pluralize renders "n singular"/"n plural".
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(n) + " " + plural
}
