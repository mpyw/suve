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
	"github.com/mpyw/suve/internal/tui/hit"
	"github.com/mpyw/suve/internal/tui/styles"
)

// Apply confirm-phase button region IDs (the results phase reuses regionClose).
const (
	applyRegionIgnore = "ignore"
	applyRegionApply  = "apply"
	applyRegionCancel = "apply-cancel"
)

// applyResultsChrome is the vertical overhead the results view reserves around its
// scrollable body so the box fits the screen: the shell's dialog border (top +
// bottom = 2 rows), the pinned "Apply results" title plus its blank spacer
// (2 rows), and the pinned blank spacer plus close hint (2 rows). The viewport
// height is capped at screenHeight−applyResultsChrome so a long fan-out result list
// scrolls inside the box while the title and close hint stay visible.
const applyResultsChrome = 6

// applyControl identifies a focusable row in the apply confirmation.
type applyControl int

const (
	applyCtrlIgnore applyControl = iota
	ctrlApply
	ctrlApplyCancel
)

// applyPhase is the dialog's stage: confirm → busy → results.
type applyPhase int

const (
	applyPhaseConfirm applyPhase = iota
	applyPhaseBusy
	applyPhaseResults
)

// applyResultsMsg carries the aggregated fan-out results back into the dialog.
type applyResultsMsg struct {
	results []data.StagingApplyResult
	err     error
}

// applyDialog confirms, runs, and reports a staged-apply. Several targets (the
// apply-all) are applied as one all-service apply, and their per-service results
// render as one coherent results view. While busy it swallows input (the #568 double-fire
// guard) and reports Busy() so the shell suppresses dismissal.
type applyDialog struct {
	// dialogLayout carries the terminal size (from the last WindowSizeMsg). The
	// results view wraps its lines to contentWidth so the box never overflows
	// horizontally, and caps the scrollable body at height−applyResultsChrome so a long
	// result list scrolls instead of clipping off-screen.
	dialogLayout

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
	// vp scrolls the results body when it is taller than the box can show; the
	// title and close hint are rendered outside it so they stay pinned.
	vp viewport.Model
	// scrollable records whether the last synced body overflowed the viewport, so
	// the close hint can advertise the scroll keys only when they do something.
	scrollable bool
	// hits maps a click on the confirm controls (ignore/apply/cancel) or the
	// results close hint to the same action their key equivalents perform.
	hits *hit.Map
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

func (d *applyDialog) Busy() bool { return d.phase == applyPhaseBusy }

// DismissCmd makes Back (Esc) on the results view close with the same
// reload+voice as enter: the results view has already applied, so a bare pop
// would leave the staging page and its badge stale. Any other phase returns nil
// so the shell bare-dismisses (confirm → cancel; busy is already suppressed).
func (d *applyDialog) DismissCmd() tea.Cmd {
	if d.phase == applyPhaseResults {
		return doneCmd("", d.summary(), true)
	}

	return nil
}

func (d *applyDialog) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.setSize(msg)
		d.syncViewport()

		return d, nil
	case applyResultsMsg:
		d.phase = applyPhaseResults
		d.results = msg.results

		if msg.err != nil {
			d.err = msg.err.Error()
		}

		d.syncViewport()

		return d, nil
	case tea.MouseWheelMsg:
		// Wheel scrolls the results body (the confirm phase has nothing to scroll).
		return d, scrollViewport(&d.vp, msg)
	case tea.MouseClickMsg:
		return d.handleClick(msg)
	case tea.KeyPressMsg:
		return d.handleKey(msg)
	}

	return d, nil
}

// handleClick reduces a click to the same action its key equivalent performs: in
// the results phase the close hint closes (like enter); in the confirm phase the
// ignore/apply/cancel controls each focus + activate. Busy swallows clicks (the
// #568 double-fire guard).
func (d *applyDialog) handleClick(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if d.phase == applyPhaseBusy {
		return d, nil
	}

	id, _, _, ok := d.hits.At(msg.X, msg.Y)
	if !ok {
		return d, nil
	}

	if d.phase == applyPhaseResults {
		if id == regionClose {
			return d, doneCmd("", d.summary(), true)
		}

		return d, nil
	}

	switch id {
	case applyRegionIgnore:
		d.focus = int(applyCtrlIgnore)
	case applyRegionApply:
		d.focus = int(ctrlApply)
	case applyRegionCancel:
		d.focus = int(ctrlApplyCancel)
	default:
		return d, nil
	}

	return d.activate()
}

func (d *applyDialog) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if d.phase == applyPhaseBusy {
		return d, nil // double-submit guard: swallow input while applying
	}

	if d.phase == applyPhaseResults {
		if key.Matches(msg, navSelect) {
			return d, doneCmd("", d.summary(), true)
		}

		// Any other key scrolls the results body (↑↓/j/k, pgup/pgdn, etc.); Esc is
		// intercepted by the shell before it reaches here, so it still closes.
		return d, scrollViewport(&d.vp, msg)
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
	case applyCtrlIgnore:
		d.ignoreConflicts = !d.ignoreConflicts
	case ctrlApply:
		d.phase = applyPhaseBusy

		return d, d.applyCmd()
	case ctrlApplyCancel:
		return d, canceledCmd
	}

	return d, nil
}

// applyCmd applies every target as one operation (data.StagingApplyAll): with
// conflict detection on, a conflict in any target rejects the whole apply, so an
// apply-all never leaves some services applied and others rejected (#982).
func (d *applyDialog) applyCmd() tea.Cmd {
	ctx := d.ctx
	targets := d.targets
	ignore := d.ignoreConflicts

	return func() tea.Msg {
		results, err := data.StagingApplyAll(ctx, targets, ignore)

		return applyResultsMsg{results: results, err: err}
	}
}

func (d *applyDialog) View() string {
	if d.phase == applyPhaseResults {
		return d.resultsView()
	}

	return d.confirmView()
}

func (d *applyDialog) confirmView() string {
	var b strings.Builder

	title := d.fit(d.styles.PaneTitle.Render(d.title))
	target := d.fit(d.styles.FieldLabel.Render("Target  ") + " " + d.targetLine)
	changes := d.fit(d.styles.FieldLabel.Render("Changes ") + " " + d.changesLine())

	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(target + "\n")
	b.WriteString(changes + "\n\n")

	if d.phase == applyPhaseBusy {
		d.hits = hit.New() // no controls while applying
		b.WriteString(d.styles.PageHint.Render("applying…"))

		return b.String()
	}

	ignoreRow := d.confirmRow(applyCtrlIgnore, checkbox(d.ignoreConflicts)+" Ignore conflicts")
	applyBtn := d.confirmRow(ctrlApply, "[ Apply ]")
	cancelBtn := d.confirmRow(ctrlApplyCancel, "[ Cancel ]")

	b.WriteString(ignoreRow)
	b.WriteString("\n\n")
	b.WriteString(applyBtn + "    " + cancelBtn)
	b.WriteString("\n\n")
	b.WriteString(d.styles.PageHint.Render("↑↓: move · space/enter: toggle/confirm · esc: cancel"))

	// The ignore row sits below the title, its blank line, the target/changes
	// lines, and their blank line; the Apply/Cancel buttons one blank line below it.
	ignoreY := lipgloss.Height(title) + 1 + lipgloss.Height(target) + lipgloss.Height(changes) + 1
	buttonY := ignoreY + lipgloss.Height(ignoreRow) + 1
	applyW := lipgloss.Width(applyBtn)
	d.hits = hit.New(
		hit.Region(applyRegionIgnore, 0, ignoreY, max(lipgloss.Width(ignoreRow), 1), 1),
		hit.Region(applyRegionApply, 0, buttonY, applyW, 1),
		hit.Region(applyRegionCancel, applyW+buttonGap, buttonY, lipgloss.Width(cancelBtn), 1),
	)

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

// syncViewport (re)builds the scrollable results body and sizes the viewport to
// min(needed, screenHeight−applyResultsChrome), so a long fan-out result list scrolls
// inside the box while the title and close hint stay pinned. It is a no-op until
// a WindowSizeMsg arrives (before that the results view renders inline, uncapped).
func (d *applyDialog) syncViewport() {
	if !d.sized() {
		return
	}

	body := d.resultsBody()
	lines := max(lipgloss.Height(body), 1)
	avail := max(d.height-applyResultsChrome, 1)
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
	body := d.resultsBody()
	if d.sized() {
		body = d.vp.View()
	}

	b.WriteString(body)
	b.WriteString("\n\n")

	hint := d.styles.PageHint.Render(d.resultsHint())
	b.WriteString(hint)

	// The close hint sits below the pinned title, its blank line, the results body,
	// and its blank line; a click on it closes like enter/esc.
	closeY := 1 + 1 + lipgloss.Height(body) + 1
	d.hits = hit.New(hit.Region(regionClose, 0, closeY, lipgloss.Width(hint), 1))

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

	// A rejected apply-all lists only the services that conflicted, so say that
	// the others were held back too.
	if len(d.targets) > 1 && d.rejected() {
		b.WriteString(d.fit(d.styles.Banner.Render("⚠ apply rejected: no service was applied")) + "\n\n")
	}

	for _, res := range d.results {
		d.writeServiceResults(&b, res)
	}

	return strings.TrimRight(b.String(), "\n")
}

// writeServiceResults appends one service's entry/tag statuses, conflicts, and
// post-apply unstage warnings.
func (d *applyDialog) writeServiceResults(b *strings.Builder, res data.StagingApplyResult) {
	// Label every block of a multi-target apply, even when a rejection reports
	// only one service.
	if len(d.targets) > 1 {
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
			b.WriteString(d.fit(d.unstageWarn(applyEntryLabel(e.Name, e.Namespace), e.UnstageError)) + "\n")
		}
	}

	for _, t := range res.Tags {
		if t.UnstageError != "" {
			b.WriteString(d.fit(d.unstageWarn(applyEntryLabel(t.Name, t.Namespace), t.UnstageError)) + "\n")
		}
	}

	b.WriteString("\n")
}

// entryResultLine renders one entry apply status.
func (d *applyDialog) entryResultLine(e data.ApplyEntryResult) string {
	if e.Error != "" {
		return d.styles.ErrorText.Render("✗ "+e.Status) + "  " + applyEntryLabel(e.Name, e.Namespace) +
			"   " + d.styles.ErrorText.Render(e.Error)
	}

	return d.styles.DiffAdded.Render("✓ "+e.Status) + "  " + applyEntryLabel(e.Name, e.Namespace)
}

// tagResultLine renders one tag apply status.
func (d *applyDialog) tagResultLine(t data.ApplyTagResult) string {
	if t.Error != "" {
		return d.styles.ErrorText.Render("✗ tags") + "  " + applyEntryLabel(t.Name, t.Namespace) +
			"   " + d.styles.ErrorText.Render(t.Error)
	}

	return d.styles.DiffAdded.Render("✓ tags") + "  " + applyEntryLabel(t.Name, t.Namespace)
}

// unstageWarn renders the "applied but could not be unstaged" warning.
func (d *applyDialog) unstageWarn(label, err string) string {
	return d.styles.Banner.Render("⚠ " + label + " applied but could not be unstaged: " + err + " — clear it manually.")
}

// rejected reports whether the apply was rejected by conflicts: some conflict
// was reported and nothing was applied or failed.
func (d *applyDialog) rejected() bool {
	applied, failed, conflicts := d.tally()

	return conflicts > 0 && applied == 0 && failed == 0
}

// summary voices the aggregated apply outcome for the status line.
func (d *applyDialog) summary() string {
	applied, failed, conflicts := d.tally()

	switch {
	case d.rejected():
		return fmt.Sprintf("Apply rejected: %d conflict(s). Re-apply with Ignore conflicts to overwrite.", conflicts)
	case failed > 0:
		return fmt.Sprintf("Applied %d, failed %d.", applied, failed)
	default:
		return "Applied " + strconv.Itoa(applied) + " staged change(s)."
	}
}

// tally counts the applied and failed entry/tag results and the conflicts
// across every service.
func (d *applyDialog) tally() (applied, failed, conflicts int) {
	for _, res := range d.results {
		conflicts += len(res.Conflicts)

		for _, e := range res.Entries {
			countApplyOutcome(e.Error, &applied, &failed)
		}

		for _, t := range res.Tags {
			countApplyOutcome(t.Error, &applied, &failed)
		}
	}

	return applied, failed, conflicts
}

// countApplyOutcome tallies a result as applied or failed.
func countApplyOutcome(errMsg string, applied, failed *int) {
	if errMsg != "" {
		*failed++
	} else {
		*applied++
	}
}

// applyEntryLabel renders a name with its namespace badge (bare name when empty).
func applyEntryLabel(name, namespace string) string {
	if namespace == "" {
		return name
	}

	return name + " [" + namespace + "]"
}
