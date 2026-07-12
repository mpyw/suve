package dialogs

import (
	"context"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// resetControl identifies a focusable row in the reset confirmation.
type resetControl int

const (
	ctrlReset resetControl = iota
	ctrlResetCancel
)

// resetResultsMsg carries the aggregated fan-out reset results back.
type resetResultsMsg struct {
	results []data.StagingResetResult
	err     error
}

// resetDialog confirms and runs a staged reset (per-service or reset-all). It
// fans out one ResetUseCase per target and voices the aggregated outcome. While
// busy it swallows input and reports Busy() so the shell suppresses dismissal.
type resetDialog struct {
	ctx     context.Context //nolint:containedctx // the reset command needs the Run context; mirrors the browser
	targets []data.StagingService
	title   string
	styles  styles.Styles

	focus int
	busy  bool
}

// ResetInput configures a reset dialog.
type ResetInput struct {
	Ctx     context.Context //nolint:containedctx // Run context threaded into the reset command; mirrors the browser
	Targets []data.StagingService
	// Title is the dialog title (e.g. "Reset staged changes — Secret" / "— all").
	Title  string
	Styles styles.Styles
}

// NewReset builds a reset dialog. Focus starts on Cancel so an accidental
// enter (e.g. an "R enter" double-tap) does not wipe staged changes — parity
// with the delete/apply confirms, which also default to a non-destructive
// control.
func NewReset(in ResetInput) Model {
	return &resetDialog{
		ctx:     in.Ctx,
		targets: in.Targets,
		title:   in.Title,
		styles:  in.Styles,
		focus:   int(ctrlResetCancel),
	}
}

func (d *resetDialog) Busy() bool { return d.busy }

func (d *resetDialog) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case resetResultsMsg:
		d.busy = false

		if msg.err != nil {
			// A hard failure may still have reset earlier fan-out targets. Close and
			// reload like a success (the app's onMutationDone), so those succeeded
			// resets refresh their badges immediately, and voice the failure on the
			// status line rather than leaving the dialog stuck open.
			return d, doneCmd("", "Reset failed: "+msg.err.Error(), true)
		}

		return d, doneCmd("", resetSummary(msg.results), true)
	case tea.KeyPressMsg:
		if d.busy {
			return d, nil // double-submit guard
		}

		return d.handleKey(msg)
	}

	return d, nil
}

func (d *resetDialog) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, navUp), key.Matches(msg, navDown):
		d.focus = 1 - d.focus
	case key.Matches(msg, navSelect):
		return d.activate()
	}

	return d, nil
}

func (d *resetDialog) activate() (Model, tea.Cmd) {
	if resetControl(d.focus) == ctrlResetCancel {
		return d, canceledCmd
	}

	d.busy = true

	return d, d.resetCmd()
}

// resetCmd fans the reset out across every target sequentially.
func (d *resetDialog) resetCmd() tea.Cmd {
	ctx := d.ctx
	targets := d.targets

	return func() tea.Msg {
		results := make([]data.StagingResetResult, 0, len(targets))

		for _, svc := range targets {
			res, err := svc.Reset(ctx)
			if err != nil {
				return resetResultsMsg{results: results, err: err}
			}

			results = append(results, res)
		}

		return resetResultsMsg{results: results}
	}
}

func (d *resetDialog) View() string {
	var b strings.Builder

	b.WriteString(d.styles.PaneTitle.Render(d.title))
	b.WriteString("\n\n")

	if d.busy {
		b.WriteString(d.styles.PageHint.Render("resetting…"))

		return b.String()
	}

	b.WriteString("Unstage every staged change for the target(s)?\n\n")
	b.WriteString(d.resetRow(ctrlReset, d.styles.ErrorText.Render("[ Reset ]")) + "    " +
		d.resetRow(ctrlResetCancel, "[ Cancel ]"))
	b.WriteString("\n\n")
	b.WriteString(d.styles.PageHint.Render("↑↓: move · enter: confirm · esc: cancel"))

	return b.String()
}

func (d *resetDialog) resetRow(c resetControl, label string) string {
	if resetControl(d.focus) == c {
		return d.styles.StatusValue.Render("▸ " + label)
	}

	return "  " + label
}

// resetSummary voices the aggregated reset outcome. A single target voices its
// exact ResetResultType; a fan-out sums the unstaged counts.
func resetSummary(results []data.StagingResetResult) string {
	if len(results) == 1 {
		return resetTypeStatus(results[0])
	}

	total := 0
	for _, r := range results {
		total += r.Count
	}

	if total == 0 {
		return "Nothing staged."
	}

	return "Unstaged " + strconv.Itoa(total) + " staged change(s)."
}

// resetTypeStatus maps a single reset result's type onto its voiced phrase.
func resetTypeStatus(r data.StagingResetResult) string {
	switch r.Type {
	case data.StagingResetUnstagedAll:
		return "Unstaged " + strconv.Itoa(r.Count) + " staged change(s)."
	case data.StagingResetNothingStaged:
		return "Nothing staged."
	case data.StagingResetUnstaged:
		return "Unstaged the staged change."
	case data.StagingResetUnstagedTag:
		return "Unstaged the staged tag change."
	case data.StagingResetRestored:
		return "Restored the staged value."
	case data.StagingResetSkipped:
		return "Skipped — value matches the current value."
	case data.StagingResetNotStaged:
		return "Not staged."
	default:
		return "Reset."
	}
}
