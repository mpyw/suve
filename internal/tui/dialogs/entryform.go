package dialogs

import (
	"context"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"golang.org/x/term"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/cli/commands/aws/param/paramtype"
	"github.com/mpyw/suve/internal/cli/editor"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// dialogContentWidth is the fixed inner width every dialog's huh form lays out
// to, so the modal size (and its goldens) stay deterministic regardless of
// terminal width.
const dialogContentWidth = 54

// isTTY reports whether the process is attached to a terminal, gating the
// $EDITOR handoff (which suspends the program to run an editor). It is a package
// variable so a test can exercise the no-TTY branch without a real terminal.
//
//nolint:gochecknoglobals // swappable TTY-detection seam for the editor handoff
var isTTY = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// editorFinishedMsg carries the $EDITOR buffer back after the editor exits.
type editorFinishedMsg struct {
	content string
	err     error
}

// ctrlEKey is the value-field $EDITOR handoff binding.
//
//nolint:gochecknoglobals // immutable dialog-local binding
var ctrlEKey = key.NewBinding(key.WithKeys("ctrl+e"))

// entryForm is the create/edit dialog. It embeds a huh form (name, type,
// namespace, value, description, mode) as a model and adds a $EDITOR handoff on
// the value field.
type entryForm struct {
	ctx     context.Context //nolint:containedctx // the mutation command needs the Run context; mirrors the browser
	mutator data.Mutator
	svcCap  capability.ServiceCapability
	service string
	styles  styles.Styles

	edit bool

	// stagedOnly hides the mode toggle and forces a staged write (the staging
	// review page's edit path); the browser leaves it false and keeps the toggle.
	stagedOnly bool

	// Bound form values.
	name        string
	namespace   string
	valueType   string
	value       string
	description string
	staged      bool

	form *huh.Form

	busy   bool
	err    string
	notice string
}

// EntryFormInput configures a create/edit dialog.
type EntryFormInput struct {
	Ctx     context.Context //nolint:containedctx // Run context threaded into the mutation command; mirrors the browser
	Mutator data.Mutator
	Service string
	Styles  styles.Styles
	// Edit switches the dialog to edit mode (name is fixed, not entered).
	Edit bool
	// Name/Namespace/Value/TypeLabel/Description seed the fields. For create,
	// Namespace seeds the App Configuration namespace default (the viewing one).
	Name      string
	Namespace string
	Value     string
	TypeLabel string
	// StagedOnly opens the dialog from a staged-only surface (the staging review
	// page): the mode toggle is hidden and the write is forced staged, so a staged
	// review can never launch an immediate write that bypasses the staging store.
	StagedOnly  bool
	Description string
}

// NewEntryForm builds a create/edit dialog. It returns the dialog and its Init
// command (the embedded huh form's Init), which the app batches on open.
func NewEntryForm(in EntryFormInput) (Model, tea.Cmd) {
	svcCap := in.Mutator.Capability()

	d := &entryForm{
		ctx:         in.Ctx,
		mutator:     in.Mutator,
		svcCap:      svcCap,
		service:     in.Service,
		styles:      in.Styles,
		edit:        in.Edit,
		name:        in.Name,
		namespace:   in.Namespace,
		valueType:   defaultTypeLabel(svcCap, in.TypeLabel),
		value:       in.Value,
		description: in.Description,
		stagedOnly:  in.StagedOnly,
		// Staged by default when the service supports staging; otherwise always
		// immediate (the mode toggle is hidden). A staged-only surface forces
		// staged regardless (its toggle is hidden too).
		staged: svcCap.HasStaging || in.StagedOnly,
	}

	cmd := d.rebuildForm()

	return d, cmd
}

// showType reports whether the typed-param Type select is offered: only the AWS
// SSM param service has a value type (App Configuration is untyped; secret has
// none) — parity with the GUI's ParamTypeOptions — AND only in immediate mode.
//
// Containment for #664: the staged write path drops the value type (staging
// apply hardcodes a plaintext String), so a staged SecureString create would
// silently apply as plaintext String. Until the systemic staged-type fix lands
// (#664), the Type select is offered only in immediate mode, where it takes
// effect; staged mode never presents a Type control that can't be honored.
func (d *entryForm) showType() bool {
	return d.svcCap.Service == serviceParam && !d.svcCap.HasNamespaces && !d.staged
}

// defaultTypeLabel picks the Type select's initial value: the seeded label when
// valid, else the canonical default ("String").
func defaultTypeLabel(svcCap capability.ServiceCapability, seed string) string {
	if svcCap.Service != serviceParam || svcCap.HasNamespaces {
		return ""
	}

	if paramtype.Validate(seed) == nil && seed != "" {
		return seed
	}

	return paramtype.String
}

// rebuildForm constructs the huh form from the current field values (so a retry
// after an error keeps what the user typed) and returns its Init command.
func (d *entryForm) rebuildForm() tea.Cmd {
	fields := make([]huh.Field, 0, 6) //nolint:mnd // at most six fields

	if !d.edit {
		fields = append(fields, huh.NewInput().Key("name").Title("Name").
			Value(&d.name).Validate(requiredField("name")))
	}

	if d.svcCap.HasNamespaces {
		fields = append(fields, d.namespaceField())
	}

	if d.showType() {
		fields = append(fields, huh.NewSelect[string]().Key("type").Title("Type").
			Options(huh.NewOptions(paramtype.Options()...)...).Value(&d.valueType))
	}

	fields = append(fields, huh.NewText().Key("value").Title("Value").Lines(4). //nolint:mnd // value textarea height
											ExternalEditor(false).Value(&d.value))

	// Description is AWS-only: the gcloud, Azure Key Vault, and App Configuration
	// writers ignore it (and the GUI never offers it), so it is shown only for a
	// service that honors it.
	if d.svcCap.HasDescription {
		fields = append(fields, huh.NewInput().Key("description").Title("Description").
			Placeholder("(optional)").Value(&d.description))
	}

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

	return d.form.Init()
}

// namespaceField builds the App Configuration namespace field. On CREATE it is an
// editable input (seeded with the viewing namespace). On EDIT it is a read-only
// note: a write targets one concrete namespace, so editing the namespace of an
// existing entry would silently retarget a DIFFERENT namespace — the field is
// disabled just as the name field is omitted on edit.
func (d *entryForm) namespaceField() huh.Field {
	if d.edit {
		return huh.NewNote().Title("Namespace").Description(namespaceDisplay(d.namespace))
	}

	return huh.NewInput().Key("namespace").Title("Namespace").
		Placeholder("(default)").Value(&d.namespace)
}

func (d *entryForm) Busy() bool { return d.busy }

func (d *entryForm) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mutationResultMsg:
		return d.onResult(msg)
	case editorFinishedMsg:
		return d.onEditorFinished(msg)
	case tea.KeyPressMsg:
		if d.busy {
			return d, nil // double-submit guard: swallow input mid-operation
		}

		if key.Matches(msg, ctrlEKey) && d.valueFieldFocused() {
			return d, d.openEditor()
		}
	}

	if d.busy {
		return d, nil
	}

	return d.forwardToForm(msg)
}

// forwardToForm drives the embedded huh form and reacts to its completion.
func (d *entryForm) forwardToForm(msg tea.Msg) (Model, tea.Cmd) {
	form, cmd := d.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		d.form = f
	}

	switch d.form.State {
	case huh.StateCompleted:
		d.notice = ""
		d.busy = true

		return d, d.submit()
	case huh.StateAborted:
		return d, canceledCmd
	case huh.StateNormal:
	}

	return d, cmd
}

// valueFieldFocused reports whether the huh value textarea currently has focus,
// so ctrl+e is intercepted only there.
func (d *entryForm) valueFieldFocused() bool {
	f := d.form.GetFocusedField()

	return f != nil && f.GetKey() == "value"
}

// submit runs the create/edit mutation off the update loop.
func (d *entryForm) submit() tea.Cmd {
	key := data.StagedKey{Name: d.name, Namespace: d.namespace}
	staged := d.staged
	value, valueType, description := d.value, d.valueType, d.description
	edit := d.edit
	mut, ctx := d.mutator, d.ctx

	return runMutation(func() (data.WriteOutcome, error) {
		if edit {
			return mut.Update(ctx, key, value, valueType, description, staged)
		}

		return mut.Create(ctx, key, value, valueType, description, staged)
	})
}

// onResult applies a mutation result: on success it emits MutationDoneMsg; on
// error it rebuilds the form so the user can fix the input and retry.
func (d *entryForm) onResult(msg mutationResultMsg) (Model, tea.Cmd) {
	d.busy = false

	if msg.err != nil {
		d.err = msg.err.Error()

		return d, d.rebuildForm()
	}

	d.err = ""
	status := entryStatus(d.edit, d.staged, msg.outcome)

	return d, doneCmd(d.service, status, d.staged)
}

// openEditor writes the current value to a temp file and hands off to the user's
// editor. The editor command is built by the shared internal/cli/editor helper,
// so the TUI and the CLI resolve VISUAL→EDITOR, honor a flag-bearing or
// space-containing editor path, and pick the same OS fallback — they cannot
// diverge.
func (d *entryForm) openEditor() tea.Cmd {
	if !isTTY() {
		d.notice = "editor needs a TTY."

		return nil
	}

	buffer := d.value

	tmp, err := os.CreateTemp("", "suve-tui-*.txt")
	if err != nil {
		d.notice = "could not open editor: " + err.Error()

		return nil
	}

	name := tmp.Name()

	if _, err := tmp.WriteString(buffer); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		d.notice = "could not open editor: " + err.Error()

		return nil
	}

	_ = tmp.Close()

	cmd := editor.Command(d.ctx, name)

	return tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		//nolint:gosec // name is the temp file this handler just created, not user input
		content, readErr := os.ReadFile(name)
		_ = os.Remove(name)

		if runErr != nil {
			return editorFinishedMsg{err: runErr}
		}

		return editorFinishedMsg{content: string(content), err: readErr}
	})
}

// onEditorFinished folds the editor buffer back into the value field: an
// unchanged buffer is a no-op ("No changes made."), otherwise the new content
// replaces the textarea's value and the form is rebuilt so the huh textarea
// buffer re-syncs (mirroring every other path).
func (d *entryForm) onEditorFinished(msg editorFinishedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		d.notice = "editor error: " + msg.err.Error()

		return d, nil
	}

	// Most editors auto-append a trailing newline; normalize it away with the same
	// round-trip-lossless rule the CLI uses, so an untouched round-trip is
	// byte-identical to the seed value and correctly reported as a no-op (instead
	// of silently mutating the value with a stray newline). d.value still holds the
	// pre-edit buffer that was written to the tmpfile.
	content := editor.Normalize(d.value, msg.content)

	if content == d.value {
		d.notice = "No changes made."

		return d, nil
	}

	d.value = content
	d.notice = "Loaded from editor."

	// Rebuild so the huh textarea re-binds to the edited value (consistent with
	// every other value-changing path; avoids a stale textarea buffer).
	return d, d.rebuildForm()
}

func (d *entryForm) View() string {
	title := "Edit " + d.name
	if !d.edit {
		title = "New " + entryNoun(d.svcCap)
	}

	var b strings.Builder

	b.WriteString(d.styles.PaneTitle.Render(title))
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

	if d.notice != "" {
		b.WriteString(d.styles.Banner.Render(d.notice))
		b.WriteString("\n")
	}

	b.WriteString(d.styles.PageHint.Render(entryHint(d.valueFieldFocused())))

	return b.String()
}

// entryHint is the bottom hint line, adding the ctrl+e affordance while the
// value field is focused.
func entryHint(onValue bool) string {
	if onValue {
		return "tab/↑↓: fields · ctrl+e: $EDITOR · enter: submit · esc: cancel"
	}

	return "tab/↑↓: fields · enter: submit · esc: cancel"
}

// namespaceDisplay renders a namespace for the read-only edit note, showing the
// null (default) namespace as "(default)" so a blank line never hides it.
func namespaceDisplay(namespace string) string {
	if namespace == "" {
		return "(default)"
	}

	return namespace
}

// entryNoun names the created item per service (App Configuration setting vs SSM
// parameter vs secret).
func entryNoun(svcCap capability.ServiceCapability) string {
	switch {
	case svcCap.Service == serviceParam && svcCap.HasNamespaces:
		return "setting"
	case svcCap.Service == serviceParam:
		return "parameter"
	default:
		return serviceSecret
	}
}

// entryStatus voices the create/edit outcome (skip/unstage/staged/applied).
func entryStatus(edit, staged bool, o data.WriteOutcome) string {
	switch {
	case o.Skipped:
		return "No change — value matches the current value; nothing staged."
	case o.Unstaged:
		return "Reverted to the base value — change auto-unstaged."
	}

	verb := "create"
	if edit {
		verb = "update"
	}

	if staged {
		return "Staged " + verb + "."
	}

	return "Applied " + verb + "."
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

// stringError is a small sentinel error type for dialog validation.
type stringError string

func (e stringError) Error() string { return string(e) }
