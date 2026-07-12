package dialogs

import (
	"context"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// Recovery-window bounds (AWS Secrets Manager: 7–30 days), with 30 the default.
const (
	minRecoveryWindow     = 7
	maxRecoveryWindow     = 30
	defaultRecoveryWindow = 30
)

// NowFunc is the clock the "recoverable until" date is computed from. It is an
// exported package variable so a golden can pin the date deterministically.
//
//nolint:gochecknoglobals // swappable clock seam for the recoverable-until date
var NowFunc = time.Now

// deleteControl identifies a focusable row in the delete dialog.
type deleteControl int

const (
	ctrlForce deleteControl = iota
	ctrlRecovery
	ctrlMode
	ctrlDelete
	ctrlCancel
)

// deleteConfirm is the delete dialog. The force row appears per HasForceDelete
// (AWS secret); the recovery-window row appears per HasRecoveryWindow but ONLY in
// staged mode — an immediate delete cannot pass a custom window (there is no
// SDK-neutral recovery-window DeleteOption, so immediate always applies AWS's
// 30-day default, matching the GUI's SecretDelete(name, force)). Force and
// recovery are also mutually exclusive (forcing hides the recovery row — CLI
// parity). The mode toggle appears only when the service supports staging.
type deleteConfirm struct {
	dialogLayout

	ctx     context.Context //nolint:containedctx // the mutation command needs the Run context; mirrors the browser
	mutator data.Mutator
	svcCap  capability.ServiceCapability
	service string
	styles  styles.Styles

	name      string
	namespace string

	force          bool
	recoveryWindow int
	staged         bool

	focus int
	busy  bool
	err   string
}

// DeleteInput configures a delete dialog.
type DeleteInput struct {
	Ctx       context.Context //nolint:containedctx // Run context threaded into the mutation command; mirrors the browser
	Mutator   data.Mutator
	Service   string
	Styles    styles.Styles
	Name      string
	Namespace string
}

// NewDeleteConfirm builds a delete dialog.
func NewDeleteConfirm(in DeleteInput) Model {
	svcCap := in.Mutator.Capability()

	return &deleteConfirm{
		ctx:            in.Ctx,
		mutator:        in.Mutator,
		svcCap:         svcCap,
		service:        in.Service,
		styles:         in.Styles,
		name:           in.Name,
		namespace:      in.Namespace,
		recoveryWindow: defaultRecoveryWindow,
		staged:         svcCap.HasStaging,
	}
}

func (d *deleteConfirm) Busy() bool { return d.busy }

// controls returns the focusable rows in order, gated by capability. The
// recovery row shows only for a staged delete (an immediate delete cannot carry a
// custom window — GUI parity) and is hidden while forcing (mutual exclusion), so
// navigation and hit-testing follow what is drawn.
func (d *deleteConfirm) controls() []deleteControl {
	var out []deleteControl

	if d.svcCap.HasForceDelete {
		out = append(out, ctrlForce)
	}

	if d.svcCap.HasRecoveryWindow && !d.force && d.staged {
		out = append(out, ctrlRecovery)
	}

	if d.svcCap.HasStaging {
		out = append(out, ctrlMode)
	}

	return append(out, ctrlDelete, ctrlCancel)
}

func (d *deleteConfirm) focused() deleteControl {
	controls := d.controls()
	if d.focus < 0 || d.focus >= len(controls) {
		return ctrlDelete
	}

	return controls[d.focus]
}

func (d *deleteConfirm) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.setSize(msg)

		return d, nil
	case mutationResultMsg:
		return d.onResult(msg)
	case tea.KeyPressMsg:
		if d.busy {
			return d, nil // double-submit guard
		}

		return d.handleKey(msg)
	}

	return d, nil
}

func (d *deleteConfirm) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, navUp):
		d.move(-1)
	case key.Matches(msg, navDown):
		d.move(1)
	case key.Matches(msg, navLeft), key.Matches(msg, navDec):
		d.adjust(-1)
	case key.Matches(msg, navRight), key.Matches(msg, navInc):
		d.adjust(1)
	case key.Matches(msg, navSelect):
		return d.activate()
	}

	return d, nil
}

// move shifts focus, keeping it in range after the control set changes.
func (d *deleteConfirm) move(delta int) {
	n := len(d.controls())
	d.focus = ((d.focus+delta)%n + n) % n
}

// focusControl points focus at c when it is drawn, so a toggle that reshapes the
// control set (force/mode showing or hiding the recovery row) keeps the same row
// focused instead of letting the index drift onto a neighbour.
func (d *deleteConfirm) focusControl(c deleteControl) {
	for i, cc := range d.controls() {
		if cc == c {
			d.focus = i

			return
		}
	}

	d.move(0)
}

// adjust changes the recovery window (when focused) within bounds.
func (d *deleteConfirm) adjust(delta int) {
	if d.focused() != ctrlRecovery {
		return
	}

	d.recoveryWindow = clampRecovery(d.recoveryWindow + delta)
}

// activate acts on the focused control: toggles force/mode, submits, or cancels.
func (d *deleteConfirm) activate() (Model, tea.Cmd) {
	switch d.focused() {
	case ctrlForce:
		d.force = !d.force
		// Toggling force changes the control set (hides/shows recovery); keep focus.
		d.focusControl(ctrlForce)
	case ctrlMode:
		d.staged = !d.staged
		// Toggling mode changes the control set (staged shows the recovery row);
		// keep focus on the mode row rather than letting the index drift.
		d.focusControl(ctrlMode)
	case ctrlDelete:
		d.busy = true

		return d, d.submit()
	case ctrlCancel:
		return d, canceledCmd
	case ctrlRecovery:
		// Recovery adjusts with left/right, not enter.
	}

	return d, nil
}

// submit runs the delete mutation off the update loop.
func (d *deleteConfirm) submit() tea.Cmd {
	key := data.StagedKey{Name: d.name, Namespace: d.namespace}
	force := d.force
	window := d.effectiveRecoveryWindow()
	staged := d.staged
	mut, ctx := d.mutator, d.ctx

	return runMutation(func() (data.WriteOutcome, error) {
		return mut.Delete(ctx, key, force, window, staged)
	})
}

// effectiveRecoveryWindow is the recovery window a staged delete records: 0 when
// forcing, when the service has no recovery window, or for an immediate delete
// (which cannot carry a custom window and always applies AWS's default), else the
// chosen value.
func (d *deleteConfirm) effectiveRecoveryWindow() int {
	if d.force || !d.svcCap.HasRecoveryWindow || !d.staged {
		return 0
	}

	return d.recoveryWindow
}

func (d *deleteConfirm) onResult(msg mutationResultMsg) (Model, tea.Cmd) {
	d.busy = false

	if msg.err != nil {
		d.err = msg.err.Error()

		return d, nil
	}

	return d, doneCmd(d.service, deleteStatus(d.staged, msg.outcome), d.staged)
}

func (d *deleteConfirm) View() string {
	var b strings.Builder

	// Header + wrapped target name. Wrapping the name to the dialog width keeps an
	// unwrapped long name (or sibling paths differing only in suffix) from clipping
	// at the screen edge, which would leave an ambiguous delete target — a safety
	// concern.
	header := d.fit(d.styles.PaneTitle.Render("Delete " + entryNoun(d.svcCap)))
	name := d.fit(clipName(d.name, d.namespace))

	b.WriteString(header)
	b.WriteString("\n\n")
	b.WriteString(name)
	b.WriteString("\n\n")

	if d.busy {
		b.WriteString(d.styles.PageHint.Render("working…"))

		return b.String()
	}

	var controls strings.Builder
	for _, c := range d.controls() {
		controls.WriteString(d.renderControl(c))
		controls.WriteString("\n")
	}

	// Wrap the hint too: it is the widest fixed line and would otherwise push the
	// box past a 60-column terminal, clipping "esc: cancel" off the edge.
	hint := d.fit(d.styles.PageHint.Render("↑↓: move · space/enter: toggle/submit · ←→: window · esc: cancel"))

	b.WriteString(controls.String())

	if d.err != "" {
		// The delete confirm cannot scroll (its controls need focus), so a long
		// provider error is wrapped and capped to whatever rows remain after the
		// name, controls, and hint — the controls and close hint always stay
		// on-screen rather than being pushed off the bottom by a tall error.
		fixed := lipgloss.Height(header) + nameSpacerRows + lipgloss.Height(name) +
			lipgloss.Height(strings.TrimRight(controls.String(), "\n")) + lipgloss.Height(hint)
		b.WriteString(d.wrapCapped(d.styles.ErrorText.Render(d.err), d.errBudget(fixed)))
		b.WriteString("\n")
	}

	b.WriteString(hint)

	return b.String()
}

// nameSpacerRows is the two blank lines the delete confirm pins around its
// header and target name (one after each), reserved when budgeting the error.
const nameSpacerRows = 2

// renderControl draws one focusable row, marking the focused one.
func (d *deleteConfirm) renderControl(c deleteControl) string {
	marker := "  "
	if d.focused() == c {
		marker = d.styles.StatusValue.Render("▸ ")
	}

	switch c {
	case ctrlForce:
		return marker + checkbox(d.force) + " Force delete (immediate, no recovery)"
	case ctrlRecovery:
		line := marker + "Recovery window   " + d.styles.StatusValue.Render(strconv.Itoa(d.recoveryWindow)+" days")
		hint := "  Recoverable until " + timeutil.FormatDate(NowFunc().AddDate(0, 0, d.recoveryWindow)) + " unless forced."

		return line + "\n" + d.styles.PageHint.Render(hint)
	case ctrlMode:
		return marker + "Mode   " + modeLabel(d.staged)
	case ctrlDelete:
		return marker + d.styles.ErrorText.Render("[ "+deleteButtonLabel(d.staged)+" ]")
	case ctrlCancel:
		return marker + "[ Cancel ]"
	default:
		return ""
	}
}

// deleteStatus voices the delete outcome.
func deleteStatus(staged bool, o data.WriteOutcome) string {
	if o.Unstaged {
		return "Removed the staged create — nothing left to delete."
	}

	if staged {
		return "Staged delete."
	}

	return "Deleted."
}

// deleteButtonLabel follows the mode (Stage vs Delete).
func deleteButtonLabel(staged bool) string {
	if staged {
		return "Stage"
	}

	return "Delete"
}

// clampRecovery keeps a recovery-window value within the AWS 7–30 bounds.
func clampRecovery(v int) int {
	return max(minRecoveryWindow, min(maxRecoveryWindow, v))
}
