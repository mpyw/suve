package dialogs

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// tagForm is the tag add/remove dialog: an action select (Add/Remove), a key
// input, a value input (used by Add), and a mode toggle. It embeds a huh form.
type tagForm struct {
	ctx     context.Context //nolint:containedctx // the mutation command needs the Run context; mirrors the browser
	mutator data.Mutator
	svcCap  capability.ServiceCapability
	service string
	styles  styles.Styles

	name      string
	namespace string

	remove   bool
	tagKey   string
	tagValue string
	staged   bool

	form *huh.Form
	busy bool
	err  string
}

// TagInput configures a tag dialog.
type TagInput struct {
	Ctx       context.Context //nolint:containedctx // Run context threaded into the mutation command; mirrors the browser
	Mutator   data.Mutator
	Service   string
	Styles    styles.Styles
	Name      string
	Namespace string
}

// NewTagForm builds a tag add/remove dialog.
func NewTagForm(in TagInput) (Model, tea.Cmd) {
	svcCap := in.Mutator.Capability()

	d := &tagForm{
		ctx:       in.Ctx,
		mutator:   in.Mutator,
		svcCap:    svcCap,
		service:   in.Service,
		styles:    in.Styles,
		name:      in.Name,
		namespace: in.Namespace,
		staged:    svcCap.HasStaging,
	}

	cmd := d.rebuildForm()

	return d, cmd
}

func (d *tagForm) rebuildForm() tea.Cmd {
	fields := []huh.Field{
		huh.NewSelect[bool]().Key("action").Title("Action").Inline(true).
			Options(huh.NewOption("Add tag", false), huh.NewOption("Remove tag", true)).
			Value(&d.remove),
		huh.NewInput().Key("tagkey").Title("Key").Value(&d.tagKey).Validate(requiredField("key")),
		huh.NewInput().Key("tagvalue").Title("Value").Placeholder("(add only)").Value(&d.tagValue),
	}

	if d.svcCap.HasStaging {
		fields = append(fields, huh.NewSelect[bool]().Key("mode").Title("Mode").Inline(true).
			Options(huh.NewOption("Stage", true), huh.NewOption("Apply immediately", false)).
			Value(&d.staged))
	}

	d.form = huh.NewForm(huh.NewGroup(fields...)).
		WithWidth(dialogContentWidth).
		WithShowHelp(false).
		WithShowErrors(true)

	return d.form.Init()
}

func (d *tagForm) Busy() bool { return d.busy }

func (d *tagForm) Update(msg tea.Msg) (Model, tea.Cmd) {
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

func (d *tagForm) submit() tea.Cmd {
	key := data.StagedKey{Name: d.name, Namespace: d.namespace}
	remove := d.remove
	tagKey, tagValue := d.tagKey, d.tagValue
	staged := d.staged
	mut, ctx := d.mutator, d.ctx

	return runMutation(func() (data.WriteOutcome, error) {
		if remove {
			return mut.RemoveTag(ctx, key, tagKey, staged)
		}

		return mut.AddTag(ctx, key, tagKey, tagValue, staged)
	})
}

func (d *tagForm) onResult(msg mutationResultMsg) (Model, tea.Cmd) {
	d.busy = false

	if msg.err != nil {
		d.err = msg.err.Error()

		return d, d.rebuildForm()
	}

	return d, doneCmd(d.service, tagStatus(d.remove, d.staged), d.staged)
}

func (d *tagForm) View() string {
	var b strings.Builder

	b.WriteString(d.styles.PaneTitle.Render("Tag — " + clipName(d.name, d.namespace)))
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

	b.WriteString(d.styles.PageHint.Render("tab/↑↓: fields · enter: submit · esc: cancel"))

	return b.String()
}

// tagStatus voices the tag outcome.
func tagStatus(remove, staged bool) string {
	verb := "tag add"
	if remove {
		verb = "tag removal"
	}

	if staged {
		return "Staged " + verb + "."
	}

	return "Applied " + verb + "."
}
