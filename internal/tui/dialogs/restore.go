package dialogs

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// restoreForm is the restore dialog: a single name input for a soft-deleted
// secret. Restore is immediate only (there is no staged restore), so it carries
// no mode toggle; it is offered only when the service HasRestore.
type restoreForm struct {
	dialogLayout

	ctx     context.Context //nolint:containedctx // the mutation command needs the Run context; mirrors the browser
	mutator data.Mutator
	service string
	styles  styles.Styles

	name string

	form *huh.Form
	busy bool
	err  string
}

// RestoreInput configures a restore dialog.
type RestoreInput struct {
	Ctx     context.Context //nolint:containedctx // Run context threaded into the mutation command; mirrors the browser
	Mutator data.Mutator
	Service string
	Styles  styles.Styles
	// Name seeds the name input (the browser's selected entry, when any).
	Name string
}

// NewRestore builds a restore dialog.
func NewRestore(in RestoreInput) (Model, tea.Cmd) {
	d := &restoreForm{
		ctx:     in.Ctx,
		mutator: in.Mutator,
		service: in.Service,
		styles:  in.Styles,
		name:    in.Name,
	}

	cmd := d.rebuildForm()

	return d, cmd
}

func (d *restoreForm) rebuildForm() tea.Cmd {
	d.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Key("name").Title("Name").Value(&d.name).Validate(requiredField("name")),
	)).
		WithWidth(dialogContentWidth).
		WithShowHelp(false).
		WithShowErrors(true)

	// Init the (re)built form, then cap its body to the known terminal size so a
	// retry after an error never renders at full natural height off-screen.
	return tea.Batch(d.form.Init(), d.syncFormSize())
}

// syncFormSize re-caps the embedded form's scrollable body to the current
// terminal size and footer (see the entry form for the full rationale).
func (d *restoreForm) syncFormSize() tea.Cmd {
	if d.form == nil || !d.sized() {
		return nil
	}

	form, cmd := d.form.Update(tea.WindowSizeMsg{Width: dialogContentWidth, Height: d.formBodyHeight()})
	if f, ok := form.(*huh.Form); ok {
		d.form = f
	}

	return cmd
}

// formBodyHeight is the height budget for the form body: the frame's inner
// height less the title, its blank spacer, and the footer (any active error plus
// the hint).
func (d *restoreForm) formBodyHeight() int {
	around := lipgloss.Height(d.header()) + titleSpacerRows + lipgloss.Height(d.footer())

	return max(d.availHeight()-around, minFormBody)
}

func (d *restoreForm) Busy() bool { return d.busy }

func (d *restoreForm) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.setSize(msg)

		return d, d.syncFormSize()
	case mutationResultMsg:
		return d.onResult(msg)
	case tea.KeyPressMsg:
		if d.busy {
			return d, nil // double-submit guard
		}
	}

	if d.busy {
		return d, nil
	}

	form, cmd := d.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		d.form = f
	}

	switch d.form.State {
	case huh.StateCompleted:
		d.busy = true

		return d, d.submit()
	case huh.StateAborted:
		return d, canceledCmd
	case huh.StateNormal:
	}

	return d, repaintFormScroll(d.form, msg, cmd)
}

func (d *restoreForm) submit() tea.Cmd {
	name := d.name
	mut, ctx := d.mutator, d.ctx

	return runMutation(func() (data.WriteOutcome, error) {
		return mut.Restore(ctx, name)
	})
}

func (d *restoreForm) onResult(msg mutationResultMsg) (Model, tea.Cmd) {
	d.busy = false

	if msg.err != nil {
		d.err = msg.err.Error()

		return d, d.rebuildForm()
	}

	return d, doneCmd(d.service, "Restored.", false)
}

func (d *restoreForm) View() string {
	var b strings.Builder

	b.WriteString(d.header())
	b.WriteString("\n\n")

	if d.busy {
		b.WriteString(d.styles.PageHint.Render("working…"))

		return b.String()
	}

	b.WriteString(d.form.View())
	b.WriteString("\n")
	b.WriteString(d.footer())

	return b.String()
}

// header renders the dialog title.
func (d *restoreForm) header() string {
	return d.fit(d.styles.PaneTitle.Render("Restore secret"))
}

// footer renders the pinned rows below the form: any active error (wrapped to the
// dialog width and capped so the form keeps at least minFormBody rows) then the
// key hint.
func (d *restoreForm) footer() string {
	parts := make([]string, 0, 2) //nolint:mnd // at most error + hint

	hint := d.styles.PageHint.Render("enter: restore · esc: cancel")

	if d.err != "" {
		budget := d.errBudget(lipgloss.Height(d.header()) + titleSpacerRows + minFormBody + lipgloss.Height(hint))
		parts = append(parts, d.wrapCapped(d.styles.ErrorText.Render(d.err), budget))
	}

	parts = append(parts, hint)

	return strings.Join(parts, "\n")
}
