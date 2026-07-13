package dialogs

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// tagForm is the tag add/remove dialog: an action select (Add/Remove), a key
// input, a value input (used by Add), and a mode toggle. It embeds a huh form.
type tagForm struct {
	dialogLayout

	ctx     context.Context //nolint:containedctx // the mutation command needs the Run context; mirrors the browser
	mutator data.Mutator
	svcCap  capability.ServiceCapability
	service string
	styles  styles.Styles

	name      string
	namespace string
	// tags is the entry's current tag set, seeding the Remove action's choices so
	// an untag can only target a tag that is actually present (#705). Empty when the
	// caller has none to offer (e.g. the staging review page, or an entry with no
	// loaded tags), in which case the Remove action is not offered at all (#761)
	// rather than luring the user into a select with nothing to pick.
	tags []data.Tag

	remove   bool
	tagKey   string // Add: the free-text key to add
	tagValue string // Add: the value to add
	// removeKey is the key chosen for removal, bound to the Remove action's select
	// (seeded from tags). It is distinct from the Add key so a typed Add key never
	// masquerades as a removable one.
	removeKey string
	staged    bool
	// stagedOnly hides the mode toggle and forces a staged tag write (the staging
	// review page's tag path); the browser leaves it false and keeps the toggle.
	stagedOnly bool
	// builtRemove records the action the current form was built for, so Update can
	// rebuild the form when the action toggles — morphing the key field between the
	// free-text Add input and the Remove select of existing tags.
	builtRemove bool

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
	// Tags is the entry's current tag set, offered as the Remove action's choices
	// (#705). Empty when the caller has none to offer, in which case the Remove
	// action is not offered (#761).
	Tags []data.Tag
	// StagedOnly opens the dialog from a staged-only surface (the staging review
	// page): the mode toggle is hidden and the tag write is forced staged.
	StagedOnly bool
}

// NewTagForm builds a tag add/remove dialog.
func NewTagForm(in TagInput) (Model, tea.Cmd) {
	svcCap := in.Mutator.Capability()

	d := &tagForm{
		ctx:        in.Ctx,
		mutator:    in.Mutator,
		svcCap:     svcCap,
		service:    in.Service,
		styles:     in.Styles,
		name:       in.Name,
		namespace:  in.Namespace,
		tags:       in.Tags,
		stagedOnly: in.StagedOnly,
		// Staged by default when the service supports staging; a staged-only surface
		// forces staged regardless (its toggle is hidden too).
		staged: svcCap.HasStaging || in.StagedOnly,
	}

	cmd := d.rebuildForm()

	return d, cmd
}

func (d *tagForm) rebuildForm() tea.Cmd {
	// Defensive: Remove is only ever offered when there are removable tags (below),
	// so d.remove cannot be true with an empty tag set — but if it somehow is (e.g.
	// a stale toggle), fall back to Add rather than rendering a dead-end. This keeps
	// removeField's select honest: it is only built over a non-empty tag set.
	if d.remove && len(d.tags) == 0 {
		d.remove = false
	}

	// Remove is offered only when the entry has a current tag set to constrain it
	// (#705 made Remove a select over existing tags, dropping the free-text
	// fallback). Two surfaces have no removable tags and so offer Add only:
	//   - a staged-only surface (the staging review page) knows only the staged
	//     deltas, never the remote tag set — removing a remote tag is done from the
	//     browser, where the tag set is visible;
	//   - an entry with no loaded tags has nothing to untag at all.
	// Offering Remove in either state is a dead-end: the user picks it and then
	// cannot select anything (#761). Gating it here mirrors how the mode toggle is
	// hidden on the staged-only surface.
	actionOpts := []huh.Option[bool]{huh.NewOption("Add tag", false)}
	if !d.stagedOnly && len(d.tags) > 0 {
		actionOpts = append(actionOpts, huh.NewOption("Remove tag", true))
	}

	fields := []huh.Field{
		huh.NewSelect[bool]().Key("action").Title("Action").Inline(true).
			Options(actionOpts...).Value(&d.remove),
	}

	// The key field morphs with the action: Add takes a free-text key + value (a
	// new tag is legitimately open-ended); Remove is constrained to the entry's
	// CURRENT tags so an untag can only target a tag that is actually present —
	// mirroring the GUI's per-chip remove — instead of a blind free-text key that
	// would invite a guaranteed stage-time/provider failure (#705).
	if d.remove {
		fields = append(fields, d.removeField())
	} else {
		fields = append(fields,
			huh.NewInput().Key("tagkey").Title("Key").Value(&d.tagKey).Validate(requiredField("key")),
			huh.NewInput().Key("tagvalue").Title("Value").Placeholder("(add only)").Value(&d.tagValue),
		)
	}

	d.builtRemove = d.remove

	// The mode toggle is offered only when staging is supported AND the dialog was
	// not launched from a staged-only surface (the staging review page), which has
	// no legitimate immediate-write escape hatch.
	if d.svcCap.HasStaging && !d.stagedOnly {
		fields = append(fields, newModeField(&d.staged))
	}

	d.form = huh.NewForm(huh.NewGroup(fields...)).
		WithWidth(dialogContentWidth).
		WithShowHelp(false).
		WithShowErrors(true)

	// Init the (re)built form, then cap its body to the known terminal size so a
	// retry after an error never renders at full natural height off-screen.
	return tea.Batch(d.form.Init(), d.syncFormSize())
}

// removeField builds the Remove action's key field: a select of the entry's
// current tags (labelled "key=value", valued by key so it routes straight to
// RemoveTag). The select binds removeKey, which huh seeds to the first tag on
// build, so a Remove always has a valid target. It is only reached with a
// non-empty tag set — Remove is not offered otherwise (#761) — so there is no
// empty-state branch to guard.
func (d *tagForm) removeField() huh.Field {
	opts := lo.Map(d.tags, func(t data.Tag, _ int) huh.Option[string] {
		return huh.NewOption(t.Key+"="+t.Value, t.Key)
	})

	return huh.NewSelect[string]().Key("removekey").Title("Tag").Options(opts...).Value(&d.removeKey)
}

// syncFormSize re-caps the embedded form's scrollable body to the current
// terminal size and footer (see the entry form for the full rationale): huh caps
// the group at min(naturalHeight, budget) so the form scrolls with the focused
// field kept in view rather than clipping the submit control off the bottom.
func (d *tagForm) syncFormSize() tea.Cmd {
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
func (d *tagForm) formBodyHeight() int {
	around := lipgloss.Height(d.header()) + titleSpacerRows + lipgloss.Height(d.footer())

	return max(d.availHeight()-around, minFormBody)
}

func (d *tagForm) Busy() bool { return d.busy }

func (d *tagForm) Update(msg tea.Msg) (Model, tea.Cmd) {
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

	// The action toggled: rebuild so the key field morphs between the free-text Add
	// input and the Remove select of existing tags. Clear any stale error first.
	if d.remove != d.builtRemove {
		d.err = ""

		return d, d.rebuildForm()
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
	// Remove routes the key chosen from the existing-tags select; Add routes the
	// free-text key + value.
	removeKey := d.removeKey
	tagKey, tagValue := d.tagKey, d.tagValue
	staged := d.staged
	mut, ctx := d.mutator, d.ctx

	return runMutation(func() (data.WriteOutcome, error) {
		if remove {
			return mut.RemoveTag(ctx, key, removeKey, staged)
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

// header renders the dialog title, wrapping the entry name to the dialog width so
// a long name does not overflow the box.
func (d *tagForm) header() string {
	return d.fit(d.styles.PaneTitle.Render("Tag — " + clipName(d.name, d.namespace)))
}

// footer renders the pinned rows below the form: any active error (wrapped to the
// dialog width and capped so the form keeps at least minFormBody rows) then the
// key hint.
func (d *tagForm) footer() string {
	parts := make([]string, 0, 2) //nolint:mnd // at most error + hint

	hint := d.styles.PageHint.Render("tab/↑↓: fields · enter: submit · esc: cancel")

	if d.err != "" {
		budget := d.errBudget(lipgloss.Height(d.header()) + titleSpacerRows + minFormBody + lipgloss.Height(hint))
		parts = append(parts, d.wrapCapped(d.styles.ErrorText.Render(d.err), budget))
	}

	parts = append(parts, hint)

	return strings.Join(parts, "\n")
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
