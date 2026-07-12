package dialogs

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// restoreForm is the restore dialog: a single name input for a soft-deleted
// secret. Restore is immediate only (there is no staged restore), so it carries
// no mode toggle; it is offered only when the service HasRestore.
type restoreForm struct {
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

	return d.form.Init()
}

func (d *restoreForm) Busy() bool { return d.busy }

func (d *restoreForm) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
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

	return d, cmd
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

	b.WriteString(d.styles.PaneTitle.Render("Restore secret"))
	b.WriteString("\n\n")

	if d.busy {
		b.WriteString(d.styles.PageHint.Render("working…"))

		return b.String()
	}

	b.WriteString(d.form.View())
	b.WriteString("\n")

	if d.err != "" {
		b.WriteString(d.styles.ErrorText.Render(d.err))
		b.WriteString("\n")
	}

	b.WriteString(d.styles.PageHint.Render("enter: restore · esc: cancel"))

	return b.String()
}
