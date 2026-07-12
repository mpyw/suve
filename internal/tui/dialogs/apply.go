package dialogs

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// dialogChrome is the column overhead the shell's dialog frame adds around the
// content (a 1-cell rounded border plus 1 cell of horizontal padding on each
// side); the results view caps its lines at width−dialogChrome so a long
// conflict / unstage-error line wraps inside the box instead of clipping its
// border.
const dialogChrome = 4

// minDialogContent floors the wrap width so a very narrow terminal still wraps
// to something legible rather than one column.
const minDialogContent = 24

// applyControl identifies a focusable row in the apply confirmation.
type applyControl int

const (
	ctrlIgnore applyControl = iota
	ctrlApply
	ctrlApplyCancel
)

// applyPhase is the dialog's stage: confirm → busy → results.
type applyPhase int

const (
	phaseConfirm applyPhase = iota
	phaseBusy
	phaseResults
)

// applyResultsMsg carries the aggregated fan-out results back into the dialog.
type applyResultsMsg struct {
	results []data.StagingApplyResult
	err     error
}

// applyDialog confirms, runs, and reports a staged-apply. It drives one
// ApplyUseCase per target service and aggregates the results client-side, so a
// global apply-all (Azure's param and secret have independent scopes) is one
// coherent results view. While busy it swallows input (the #568 double-fire
// guard) and reports Busy() so the shell suppresses dismissal.
type applyDialog struct {
	ctx        context.Context //nolint:containedctx // the apply command needs the Run context; mirrors the browser
	targets    []data.StagingService
	targetLine string
	entryCount int
	tagCount   int
	styles     styles.Styles

	ignoreConflicts bool
	focus           int
	phase           applyPhase
	results         []data.StagingApplyResult
	err             string
	title           string
	// width is the terminal width (from the last WindowSizeMsg); the results view
	// wraps its lines to width−dialogChrome so the box never overflows the screen.
	width int
}

// ApplyInput configures an apply dialog.
type ApplyInput struct {
	Ctx context.Context //nolint:containedctx // Run context threaded into the apply command; mirrors the browser
	// Targets are the services to apply (one for per-service, all for apply-all).
	Targets []data.StagingService
	// TargetLine is the resolved target identity string (account/region, project,
	// or vault/store) shown on the confirmation — parity with the CLI prompt.
	TargetLine string
	// Title is the dialog title (e.g. "Apply staged changes — Param" / "— all").
	Title string
	// EntryCount / TagCount are the staged totals across the targets.
	EntryCount int
	TagCount   int
	Styles     styles.Styles
}

// NewApply builds an apply dialog.
func NewApply(in ApplyInput) Model {
	return &applyDialog{
		ctx:        in.Ctx,
		targets:    in.Targets,
		targetLine: in.TargetLine,
		entryCount: in.EntryCount,
		tagCount:   in.TagCount,
		styles:     in.Styles,
		title:      in.Title,
	}
}

func (d *applyDialog) Busy() bool { return d.phase == phaseBusy }

// DismissCmd makes Back (Esc) on the results view close with the same
// reload+voice as enter: the results view has already applied, so a bare pop
// would leave the staging page and its badge stale. Any other phase returns nil
// so the shell bare-dismisses (confirm → cancel; busy is already suppressed).
func (d *applyDialog) DismissCmd() tea.Cmd {
	if d.phase == phaseResults {
		return doneCmd("", d.summary(), true)
	}

	return nil
}

func (d *applyDialog) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width

		return d, nil
	case applyResultsMsg:
		d.phase = phaseResults
		d.results = msg.results

		if msg.err != nil {
			d.err = msg.err.Error()
		}

		return d, nil
	case tea.KeyPressMsg:
		return d.handleKey(msg)
	}

	return d, nil
}

// contentWidth is the inner width the results box may fill: the terminal width
// less the shell's dialog frame, floored so a narrow terminal still wraps. Zero
// (before the first WindowSizeMsg) means "don't wrap".
func (d *applyDialog) contentWidth() int {
	if d.width <= 0 {
		return 0
	}

	return max(d.width-dialogChrome, minDialogContent)
}

// fit wraps one already-styled result line to the content width when it would
// overflow, so a long conflict / unstage-error / error line stays inside the box
// instead of pushing the border off-screen. Short lines pass through untouched
// so the box keeps its natural width when nothing wraps.
func (d *applyDialog) fit(line string) string {
	if w := d.contentWidth(); w > 0 && lipgloss.Width(line) > w {
		return lipgloss.NewStyle().Width(w).Render(line)
	}

	return line
}

func (d *applyDialog) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if d.phase == phaseBusy {
		return d, nil // double-submit guard: swallow input while applying
	}

	if d.phase == phaseResults {
		if key.Matches(msg, navSelect) {
			return d, doneCmd("", d.summary(), true)
		}

		return d, nil
	}

	return d.handleConfirmKey(msg)
}

func (d *applyDialog) handleConfirmKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, navUp):
		d.move(-1)
	case key.Matches(msg, navDown):
		d.move(1)
	case key.Matches(msg, navSelect):
		return d.activate()
	}

	return d, nil
}

func (d *applyDialog) move(delta int) {
	const n = 3 // ignore, apply, cancel

	d.focus = ((d.focus+delta)%n + n) % n
}

func (d *applyDialog) activate() (Model, tea.Cmd) {
	switch applyControl(d.focus) {
	case ctrlIgnore:
		d.ignoreConflicts = !d.ignoreConflicts
	case ctrlApply:
		d.phase = phaseBusy

		return d, d.applyCmd()
	case ctrlApplyCancel:
		return d, canceledCmd
	}

	return d, nil
}

// applyCmd fans the apply out across every target sequentially (one goroutine,
// so no shared state races) and aggregates the per-service results.
func (d *applyDialog) applyCmd() tea.Cmd {
	ctx := d.ctx
	targets := d.targets
	ignore := d.ignoreConflicts

	return func() tea.Msg {
		results := make([]data.StagingApplyResult, 0, len(targets))

		for _, svc := range targets {
			res, err := svc.Apply(ctx, ignore)
			if err != nil {
				return applyResultsMsg{results: results, err: err}
			}

			results = append(results, res)
		}

		return applyResultsMsg{results: results}
	}
}

func (d *applyDialog) View() string {
	if d.phase == phaseResults {
		return d.resultsView()
	}

	return d.confirmView()
}

func (d *applyDialog) confirmView() string {
	var b strings.Builder

	b.WriteString(d.styles.PaneTitle.Render(d.title))
	b.WriteString("\n\n")
	b.WriteString(d.styles.FieldLabel.Render("Target  ") + " " + d.targetLine + "\n")
	b.WriteString(d.styles.FieldLabel.Render("Changes ") + " " + d.changesLine() + "\n\n")

	if d.phase == phaseBusy {
		b.WriteString(d.styles.PageHint.Render("applying…"))

		return b.String()
	}

	b.WriteString(d.confirmRow(ctrlIgnore, checkbox(d.ignoreConflicts)+" Ignore conflicts"))
	b.WriteString("\n\n")
	b.WriteString(d.confirmRow(ctrlApply, "[ Apply ]") + "    " + d.confirmRow(ctrlApplyCancel, "[ Cancel ]"))
	b.WriteString("\n\n")
	b.WriteString(d.styles.PageHint.Render("↑↓: move · space/enter: toggle/confirm · esc: cancel"))

	return b.String()
}

// confirmRow renders a focusable confirm control, marking the focused one.
func (d *applyDialog) confirmRow(c applyControl, label string) string {
	if applyControl(d.focus) == c {
		return d.styles.StatusValue.Render("▸ " + label)
	}

	return "  " + label
}

// changesLine renders the "N entries · M tag change(s)" summary.
func (d *applyDialog) changesLine() string {
	return pluralize(d.entryCount, "entry", "entries") + " · " + pluralize(d.tagCount, "tag change", "tag changes")
}

func (d *applyDialog) resultsView() string {
	var b strings.Builder

	b.WriteString(d.styles.PaneTitle.Render("Apply results"))
	b.WriteString("\n\n")

	if d.err != "" {
		b.WriteString(d.fit(d.styles.ErrorText.Render(d.err)) + "\n\n")
	}

	for _, res := range d.results {
		d.writeServiceResults(&b, res)
	}

	b.WriteString(d.styles.PageHint.Render("enter/esc: close"))

	return b.String()
}

// writeServiceResults appends one service's entry/tag statuses, conflicts, and
// post-apply unstage warnings.
func (d *applyDialog) writeServiceResults(b *strings.Builder, res data.StagingApplyResult) {
	if len(d.results) > 1 {
		b.WriteString(d.styles.PaneTitle.Render(res.ServiceLabel) + "\n")
	}

	for _, e := range res.Entries {
		b.WriteString(d.fit(d.entryResultLine(e)) + "\n")
	}

	for _, t := range res.Tags {
		b.WriteString(d.fit(d.tagResultLine(t)) + "\n")
	}

	for _, c := range res.Conflicts {
		b.WriteString(d.fit(d.styles.Banner.Render("⚠ conflict: "+c+
			" was modified remotely after staging — re-apply with \"Ignore conflicts\" to overwrite.")) + "\n")
	}

	for _, e := range res.Entries {
		if e.UnstageError != "" {
			b.WriteString(d.fit(d.unstageWarn(entryLabel(e.Name, e.Namespace), e.UnstageError)) + "\n")
		}
	}

	for _, t := range res.Tags {
		if t.UnstageError != "" {
			b.WriteString(d.fit(d.unstageWarn(entryLabel(t.Name, t.Namespace), t.UnstageError)) + "\n")
		}
	}

	b.WriteString("\n")
}

// entryResultLine renders one entry apply status.
func (d *applyDialog) entryResultLine(e data.ApplyEntryResult) string {
	if e.Error != "" {
		return d.styles.ErrorText.Render("✗ "+e.Status) + "  " + entryLabel(e.Name, e.Namespace) +
			"   " + d.styles.ErrorText.Render(e.Error)
	}

	return d.styles.DiffAdded.Render("✓ "+e.Status) + "  " + entryLabel(e.Name, e.Namespace)
}

// tagResultLine renders one tag apply status.
func (d *applyDialog) tagResultLine(t data.ApplyTagResult) string {
	if t.Error != "" {
		return d.styles.ErrorText.Render("✗ tags") + "  " + entryLabel(t.Name, t.Namespace) +
			"   " + d.styles.ErrorText.Render(t.Error)
	}

	return d.styles.DiffAdded.Render("✓ tags") + "  " + entryLabel(t.Name, t.Namespace)
}

// unstageWarn renders the "applied but could not be unstaged" warning.
func (d *applyDialog) unstageWarn(label, err string) string {
	return d.styles.Banner.Render("⚠ " + label + " applied but could not be unstaged: " + err + " — clear it manually.")
}

// summary voices the aggregated apply outcome for the status line.
func (d *applyDialog) summary() string {
	applied, failed, conflicts := 0, 0, 0

	for _, res := range d.results {
		conflicts += len(res.Conflicts)

		for _, e := range res.Entries {
			countOutcome(e.Error, &applied, &failed)
		}

		for _, t := range res.Tags {
			countOutcome(t.Error, &applied, &failed)
		}
	}

	switch {
	case conflicts > 0 && applied == 0 && failed == 0:
		return fmt.Sprintf("Apply rejected: %d conflict(s). Re-apply with Ignore conflicts to overwrite.", conflicts)
	case failed > 0:
		return fmt.Sprintf("Applied %d, failed %d.", applied, failed)
	default:
		return "Applied " + strconv.Itoa(applied) + " staged change(s)."
	}
}

// countOutcome tallies a result as applied or failed.
func countOutcome(errMsg string, applied, failed *int) {
	if errMsg != "" {
		*failed++
	} else {
		*applied++
	}
}

// entryLabel renders a name with its namespace badge (bare name when empty).
func entryLabel(name, namespace string) string {
	if namespace == "" {
		return name
	}

	return name + " [" + namespace + "]"
}

// pluralize renders "n singular"/"n plural".
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}

	return strconv.Itoa(n) + " " + plural
}
