package dialogs

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
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

// resultsChrome is the vertical overhead the results view reserves around its
// scrollable body so the box fits the screen: the shell's dialog border (top +
// bottom = 2 rows), the pinned "Apply results" title plus its blank spacer
// (2 rows), and the pinned blank spacer plus close hint (2 rows). The viewport
// height is capped at screenHeight−resultsChrome so a long fan-out result list
// scrolls inside the box while the title and close hint stay visible.
const resultsChrome = 6

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
	// width / height are the terminal size (from the last WindowSizeMsg). The
	// results view wraps its lines to width−dialogChrome so the box never overflows
	// horizontally, and caps the scrollable body at height−resultsChrome so a long
	// result list scrolls instead of clipping off-screen.
	width  int
	height int
	// vp scrolls the results body when it is taller than the box can show; the
	// title and close hint are rendered outside it so they stay pinned.
	vp viewport.Model
	// scrollable records whether the last synced body overflowed the viewport, so
	// the close hint can advertise the scroll keys only when they do something.
	scrollable bool
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
		vp:         viewport.New(),
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
		d.width, d.height = msg.Width, msg.Height
		d.syncViewport()

		return d, nil
	case applyResultsMsg:
		d.phase = phaseResults
		d.results = msg.results

		if msg.err != nil {
			d.err = msg.err.Error()
		}

		d.syncViewport()

		return d, nil
	case tea.MouseWheelMsg:
		// Wheel scrolls the results body (the confirm phase has nothing to scroll).
		var cmd tea.Cmd

		d.vp, cmd = d.vp.Update(msg)

		return d, cmd
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

		// Any other key scrolls the results body (↑↓/j/k, pgup/pgdn, etc.); Esc is
		// intercepted by the shell before it reaches here, so it still closes.
		var cmd tea.Cmd

		d.vp, cmd = d.vp.Update(msg)

		return d, cmd
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

// sized reports whether a WindowSizeMsg has arrived, so the results view knows
// whether it may cap the body to the screen (a viewport with a zero width/height
// renders nothing).
func (d *applyDialog) sized() bool { return d.width > 0 && d.height > 0 }

// syncViewport (re)builds the scrollable results body and sizes the viewport to
// min(needed, screenHeight−resultsChrome), so a long fan-out result list scrolls
// inside the box while the title and close hint stay pinned. It is a no-op until
// a WindowSizeMsg arrives (before that the results view renders inline, uncapped).
func (d *applyDialog) syncViewport() {
	if !d.sized() {
		return
	}

	body := d.resultsBody()
	lines := max(lipgloss.Height(body), 1)
	avail := max(d.height-resultsChrome, 1)
	height := min(lines, avail)

	d.scrollable = lines > height
	d.vp.SetWidth(d.contentWidth())
	d.vp.SetHeight(height)
	d.vp.SetContent(body)
}

func (d *applyDialog) resultsView() string {
	var b strings.Builder

	b.WriteString(d.styles.PaneTitle.Render("Apply results"))
	b.WriteString("\n\n")

	// Once sized, the body scrolls inside the viewport; before the first
	// WindowSizeMsg it renders inline (uncapped) so a size-less unit render still
	// shows every line.
	if d.sized() {
		b.WriteString(d.vp.View())
	} else {
		b.WriteString(d.resultsBody())
	}

	b.WriteString("\n\n")
	b.WriteString(d.styles.PageHint.Render(d.resultsHint()))

	return b.String()
}

// resultsHint pins the close hint, advertising the scroll keys only when the body
// actually overflows the viewport.
func (d *applyDialog) resultsHint() string {
	if d.scrollable {
		return "↑↓/pgup/pgdn: scroll · enter/esc: close"
	}

	return "enter/esc: close"
}

// resultsBody renders the scrollable results content: the hard-failure banner (if
// any) followed by every service's entry/tag statuses, conflicts, and unstage
// warnings. The trailing blank each service block leaves is trimmed so the pinned
// close hint sits one blank line below the body.
func (d *applyDialog) resultsBody() string {
	var b strings.Builder

	if d.err != "" {
		b.WriteString(d.fit(d.styles.ErrorText.Render(d.err)) + "\n\n")
	}

	for _, res := range d.results {
		d.writeServiceResults(&b, res)
	}

	return strings.TrimRight(b.String(), "\n")
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
