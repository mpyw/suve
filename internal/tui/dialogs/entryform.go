package dialogs

import (
	"context"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/cli/commands/aws/param/paramtype"
	"github.com/mpyw/suve/internal/cli/editor"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// dialogContentWidth is the fixed inner width every dialog's huh form lays out
// to, so the modal size (and its goldens) stay deterministic regardless of
// terminal width. It fits the minimum supported 60-column terminal (60 −
// dialogChrome = 56 ≥ 54).
const dialogContentWidth = 54

// minFormBody floors the embedded form's scrollable body height so a very short
// terminal still shows at least a field or two (the rest scrolls into view)
// rather than collapsing the form to nothing.
const minFormBody = 3

// titleSpacerRows is the blank line the form dialogs draw between the title and
// the form body; it is reserved when budgeting the body's scrollable height.
const titleSpacerRows = 1

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

// Field keys for the two multi-line fields, shared by rebuildForm and the $EDITOR
// handoff so it knows which field it is editing.
const (
	fieldKeyValue       = "value"
	fieldKeyDescription = "description"
)

// editorKey is the value-field $EDITOR handoff binding. It is ctrl+o ("open"), not
// ctrl+e: bubbles text inputs bind ctrl+e to end-of-line (readline), so ctrl+e here
// would shadow that motion inside the Value textarea; ctrl+o is unbound by the text
// input and is a portable control character (unlike ctrl+. and friends, which only
// enhanced-keyboard terminals report).
//
//nolint:gochecknoglobals // immutable dialog-local binding
var editorKey = key.NewBinding(key.WithKeys("ctrl+o"))

// entryForm is the create/edit dialog. It embeds a huh form (name, type,
// namespace, value, description, mode) as a model and adds a $EDITOR handoff on
// the value field.
type entryForm struct {
	dialogLayout

	ctx     context.Context //nolint:containedctx // the mutation command needs the Run context; mirrors the browser
	mutator data.Mutator
	svcCap  capability.ServiceCapability
	service string
	styles  styles.Styles

	edit bool

	// stagedOnly hides the mode toggle and forces a staged write (the staging
	// review page's edit path); the browser leaves it false and keeps the toggle.
	stagedOnly bool

	// deleteStagedKeys is the set of (name, namespace) keys staged for deletion,
	// used by the create name validator to reject a delete-staged name inline
	// rather than dead-ending on the reducer's post-submit error (#692).
	deleteStagedKeys map[data.StagedKey]struct{}

	// Bound form values.
	name        string
	namespace   string
	valueType   string
	value       string
	description string
	staged      bool

	// initName/initNamespace/initValue/initDescription snapshot the seeded field
	// values at construction so the discard guard (#790) can tell a DIRTY form
	// (a free-text field changed from its seed) from a clean one. Only the
	// text-bearing fields are tracked — the Type/Mode selects seed to sensible
	// defaults and carry no typed data to lose.
	initName        string
	initNamespace   string
	initValue       string
	initDescription string
	// armed records that a first Esc on a dirty form has shown the discard notice;
	// a second consecutive Esc then discards. Any other key resets it.
	armed bool

	form *huh.Form
	// confirmOK backs the final "[ OK ]" submit button (a single-affirmative huh
	// Confirm): the user tabs to it and presses Enter to complete the form, so a
	// multi-line Value/Description (where Enter inserts a newline) has an explicit,
	// visible way out. The bool itself is inert — the single OK button always
	// completes; Esc cancels.
	confirmOK bool
	// editorField records which multi-line field ("value"/"description") the $EDITOR
	// handoff is editing, so onEditorFinished writes the returned buffer back to the
	// right one.
	editorField string

	// confirming/confirm drive the Stage/Apply popup shown before the write
	// commits (replacing the old inline Mode toggle). While confirming is true the
	// dialog routes keys to confirm and renders it in place of the form.
	confirming bool
	confirm    modeConfirm

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
	// DeleteStagedKeys lets a create dialog reject a name already staged for
	// deletion with an inline validation error (#692). Unset for an edit.
	DeleteStagedKeys map[data.StagedKey]struct{}
}

// NewEntryForm builds a create/edit dialog. It returns the dialog and its Init
// command (the embedded huh form's Init), which the app batches on open.
func NewEntryForm(in EntryFormInput) (Model, tea.Cmd) {
	svcCap := in.Mutator.Capability()

	d := &entryForm{
		ctx:              in.Ctx,
		mutator:          in.Mutator,
		svcCap:           svcCap,
		service:          in.Service,
		styles:           in.Styles,
		edit:             in.Edit,
		name:             in.Name,
		namespace:        in.Namespace,
		valueType:        defaultTypeLabel(svcCap, in.TypeLabel),
		value:            in.Value,
		description:      in.Description,
		stagedOnly:       in.StagedOnly,
		deleteStagedKeys: in.DeleteStagedKeys,
		// Staged by default when the service supports staging; otherwise always
		// immediate (the mode toggle is hidden). A staged-only surface forces
		// staged regardless (its toggle is hidden too).
		staged: svcCap.HasStaging || in.StagedOnly,
		// Snapshot the seeded free-text values for the discard guard (#790).
		initName:        in.Name,
		initNamespace:   in.Namespace,
		initValue:       in.Value,
		initDescription: in.Description,
	}

	cmd := d.rebuildForm()

	return d, cmd
}

// showType reports whether the typed-param Type select is offered: only the AWS
// SSM param service has a value type (App Configuration is untyped; secret has
// none) — parity with the GUI's ParamTypeOptions. It does NOT depend on the mode
// toggle, so the select is reachable for a staged create and flows through to
// apply (the #664/#680 fix); the immediate path maps it via paramtype.Parse and
// the staged create carries it into the staging store.
//
// It is hidden on a staged-only surface (the staging review page's edit): there
// the write is always a staged edit, which preserves the existing type rather
// than taking a new one, and the dialog cannot seed the entry's current type — so
// a Type control there could neither be honored nor shown accurately.
func (d *entryForm) showType() bool {
	return d.svcCap.Service == serviceParam && !d.svcCap.HasNamespaces && !d.stagedOnly
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
			Value(&d.name).Validate(d.nameValidator()))
	}

	if d.svcCap.HasNamespaces {
		fields = append(fields, d.namespaceField())
	}

	if d.showType() {
		fields = append(fields, huh.NewSelect[string]().Key("type").Title("Type").
			Options(huh.NewOptions(paramtype.Options()...)...).Value(&d.valueType))
	}

	fields = append(fields, huh.NewText().Key(fieldKeyValue).Title("Value").Lines(4). //nolint:mnd // value textarea height
												ExternalEditor(false).Value(&d.value))

	// Description is a free-text field the AWS (Parameter Store + Secrets Manager)
	// and Google Cloud (stored as the description annotation) writers honor; Azure
	// has no description concept, so it is shown only for a service that supports it.
	// It is multi-line like Value (both back onto free-text that accepts newlines),
	// with the same ctrl+o $EDITOR handoff.
	if d.svcCap.HasDescription {
		fields = append(fields, huh.NewText().Key(fieldKeyDescription).Title("Description").Lines(3). //nolint:mnd // description textarea height
														ExternalEditor(false).Placeholder("(optional)").Value(&d.description))
	}

	// The final "[ OK ]" button completes the form. With multi-line Value/Description
	// fields (Enter inserts a newline), Tab walks to this button and Enter submits —
	// an explicit, visible way out that does not collide with in-field newlines. A
	// single-affirmative Confirm renders just the button; Esc still cancels. The
	// Stage/Apply choice is the mode-confirm popup opened after completion (see
	// beginSubmit), not an inline field.
	// Label the button "[ OK ]" (bracketed, like the custom dialogs' [ Delete ]/
	// [ Cancel ]) so it reads as a button, not bare text.
	fields = append(fields, huh.NewConfirm().Key("ok").Affirmative("[ OK ]").Negative("").Value(&d.confirmOK))

	d.form = huh.NewForm(huh.NewGroup(fields...)).
		WithWidth(dialogContentWidth).
		WithShowHelp(false).
		WithShowErrors(true).
		WithKeyMap(formKeyMap()).
		WithTheme(entryFormTheme())

	// Init the (re)built form, then immediately cap its body to the known
	// terminal size so a retry after an error — or the initial build once the
	// size has been seeded — never renders at full natural height off-screen.
	return tea.Batch(d.form.Init(), d.syncFormSize())
}

// formKeyMap is the huh keymap the create/edit form uses so the multi-line Value
// and Description fields can hold newlines (#791): in a huh Text field Enter inserts
// a newline (NewLine), so it cannot double as "next field". Tab advances between
// fields, and the form is completed from the final "[ OK ]" button (Enter) — never
// from a text field, so a multi-line field's Submit is disabled and Enter there only
// ever inserts a newline. Every single-line field keeps huh's defaults, so Enter
// advances there. (No dialog-level submit key: ctrl+s was terminal XOFF and is gone;
// the caret motions ctrl+a/ctrl+e now stay with the text input.)
func formKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Text.NewLine = key.NewBinding(key.WithKeys("enter", "ctrl+j"), key.WithHelp("enter", "new line"))
	km.Text.Next = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next"))
	// A multi-line field is never the last field (the OK button is), so its Submit
	// never fires; disable it so Enter there only ever inserts a newline.
	km.Text.Submit = key.NewBinding(key.WithDisabled())
	// Disable huh's built-in ctrl+e "open editor" binding. huh only disables it via
	// Text.KeyBinds() (the help path), which never runs while WithShowHelp(false),
	// so ExternalEditor(false) alone leaves ctrl+e live — it would launch huh's
	// default "nano" and shadow the textarea's ctrl+e (readline end-of-line). We own
	// the $EDITOR handoff on ctrl+o (see editorKey), so ctrl+e is freed to reach the
	// textarea as end-of-line.
	km.Text.Editor = key.NewBinding(key.WithDisabled())

	return km
}

// entryFormTheme is huh's default (Charm) theme with only the "[ OK ]" button
// restyled so it inverts ONLY while focused. huh's single-affirmative Confirm
// always renders the affirmative with the field group's FocusedButton, and the
// default theme copies Blurred = Focused, so the button never changed on focus (you
// could not tell it was selected). Overriding the focused button to reverse-video
// and the blurred one to plain gives the single-button form a clear focus cue. It
// uses Reverse (no hard-coded colors), so it swaps whatever fg/bg is in effect and
// reads well in both light and dark terminals; ThemeCharm(isDark) still supplies
// every other style, so the rest of the form is unchanged.
func entryFormTheme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		s := huh.ThemeCharm(isDark)
		s.Focused.FocusedButton = lipgloss.NewStyle().Reverse(true).Padding(0, 1)
		s.Blurred.FocusedButton = lipgloss.NewStyle().Padding(0, 1)

		return s
	})
}

// syncFormSize re-caps the embedded form's scrollable body to the current
// terminal size and footer. It forwards a height-reduced WindowSizeMsg so huh
// caps the group at min(naturalHeight, budget): a form that fits renders whole,
// a taller one scrolls with the focused field kept in view, so the submit
// control and the pinned hint never clip off the bottom at the minimum size. It
// returns any redraw command the resize produces, and is a no-op (nil) until the
// form exists and a WindowSizeMsg has arrived.
func (d *entryForm) syncFormSize() tea.Cmd {
	if d.form == nil || !d.sized() {
		return nil
	}

	form, cmd := d.form.Update(tea.WindowSizeMsg{Width: dialogContentWidth, Height: d.formBodyHeight()})
	if f, ok := form.(*huh.Form); ok {
		d.form = f
	}

	return cmd
}

// formBodyHeight is the height budget the embedded form's scrollable body gets:
// the frame's inner height less the fixed rows this View pins around the form
// (the title and its blank spacer above, the footer — any active error/notice
// plus the hint — below).
func (d *entryForm) formBodyHeight() int {
	around := lipgloss.Height(d.header()) + titleSpacerRows + lipgloss.Height(d.footer())

	return max(d.availHeight()-around, minFormBody)
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
	case tea.WindowSizeMsg:
		d.setSize(msg)

		return d, d.syncFormSize()
	case mutationResultMsg:
		return d.onResult(msg)
	case editorFinishedMsg:
		return d.onEditorFinished(msg)
	case tea.KeyPressMsg:
		if d.busy {
			return d, nil // double-submit guard: swallow input mid-operation
		}

		// The Stage/Apply popup owns every key while it is open (esc there goes back
		// to the form, not the discard guard).
		if d.confirming {
			return d.updateConfirm(msg)
		}

		if key.Matches(msg, escKey) {
			return d.handleEsc()
		}

		// Any other key resets the discard-armed state so a later single Esc
		// re-arms (a stray Esc after typing does not silently discard).
		d.disarm()

		if key.Matches(msg, editorKey) && d.multilineFieldKey() != "" {
			return d, d.openEditor()
		}
	}

	if d.busy {
		return d, nil
	}

	return d.forwardToForm(msg)
}

// InterceptEsc opts the form into the shell's discard guard (#790): the shell
// forwards Esc into Update (see handleEsc) rather than bare-popping the dialog.
func (*entryForm) InterceptEsc() bool { return true }

// handleEsc implements the discard guard: on a DIRTY form the first Esc arms a
// confirmation (shows the notice, stays open); a second consecutive Esc — or any
// Esc on a clean/unchanged form — discards (CanceledMsg closes it).
func (d *entryForm) handleEsc() (Model, tea.Cmd) {
	if d.dirty() && !d.armed {
		d.armed = true
		d.notice = discardNotice

		return d, d.syncFormSize()
	}

	return d, canceledCmd
}

// dirty reports whether any free-text field diverges from its seeded value — a
// non-empty entry on create, or a changed value/description/etc. on edit.
func (d *entryForm) dirty() bool {
	return d.name != d.initName ||
		d.namespace != d.initNamespace ||
		d.value != d.initValue ||
		d.description != d.initDescription
}

// disarm clears the discard-armed state (and its notice) after any non-Esc key,
// so the two Escs must be consecutive.
func (d *entryForm) disarm() {
	if d.armed {
		d.armed = false
		d.notice = ""
	}
}

// beginSubmit is the submit path, reached when the huh form completes (Enter on the
// "[ OK ]" button). It opens the Stage/Apply popup when the service offers a genuine
// choice, otherwise runs the write directly (a service without staging, or a
// staged-only surface — both have a single fixed mode). huh has already validated
// every field by the time the form completes, so no re-validation is needed here.
func (d *entryForm) beginSubmit() (Model, tea.Cmd) {
	d.err = ""
	d.notice = ""

	if d.svcCap.HasStaging && !d.stagedOnly {
		d.confirm = newModeConfirm(d.confirmTitle(), d.staged)
		d.confirming = true

		return d, nil
	}

	d.busy = true

	return d, d.submit()
}

// updateConfirm folds a key press into the open Stage/Apply popup: enter commits
// with the chosen mode, esc returns to the form (rebuilding it when a huh
// StateCompleted got us here, so the form is editable again rather than instantly
// re-completing). Selection keys just repaint.
func (d *entryForm) updateConfirm(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch d.confirm.Update(msg) {
	case confirmExecute:
		d.staged = d.confirm.staged
		d.confirming = false
		d.busy = true

		return d, d.submit()
	case confirmBack:
		d.confirming = false
		if d.form.State == huh.StateCompleted {
			return d, d.rebuildForm()
		}

		return d, d.syncFormSize()
	case confirmNone:
	}

	return d, nil
}

// confirmTitle is the popup's title: the same "New …"/"Edit …" line the form
// header shows, so the popup keeps the create/edit context.
func (d *entryForm) confirmTitle() string {
	if d.edit {
		return "Edit " + d.name
	}

	return "New " + entryNoun(d.svcCap)
}

// forwardToForm drives the embedded huh form and reacts to its completion.
func (d *entryForm) forwardToForm(msg tea.Msg) (Model, tea.Cmd) {
	form, cmd := d.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		d.form = f
	}

	switch d.form.State {
	case huh.StateCompleted:
		// Enter on the final "[ OK ]" button completes the form; route it through the
		// Stage/Apply popup (or a direct write when there is no choice).
		return d.beginSubmit()
	case huh.StateAborted:
		return d, canceledCmd
	case huh.StateNormal:
	}

	return d, cmd
}

// multilineFieldKey returns the key of the focused multi-line field ("value" or
// "description"), or "" when neither is focused — so the ctrl+o $EDITOR handoff is
// intercepted only on a multi-line field and knows which one to edit.
func (d *entryForm) multilineFieldKey() string {
	f := d.form.GetFocusedField()
	if f == nil {
		return ""
	}

	switch f.GetKey() {
	case fieldKeyValue, fieldKeyDescription:
		return f.GetKey()
	default:
		return ""
	}
}

// submit runs the create/edit mutation off the update loop.
func (d *entryForm) submit() tea.Cmd {
	key := data.StagedKey{Name: d.name, Namespace: d.namespace}
	staged := d.staged
	// Pass the value type only when a Type control was actually offered. When it
	// was not (a secret, an App Configuration setting, or a staging-review edit
	// that cannot seed the entry's current type), an empty label signals "no
	// explicit type" so the staged edit preserves the existing type instead of
	// forcing the select's default and downgrading it.
	valueType := ""
	if d.showType() {
		valueType = d.valueType
	}

	value, description := d.value, d.description
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

// openEditor writes the focused multi-line field's current text to a temp file and
// hands off to the user's editor, recording which field (value/description) so the
// result is folded back into the right one. The editor command is built by the
// shared internal/cli/editor helper, so the TUI and the CLI resolve VISUAL→EDITOR,
// honor a flag-bearing or space-containing editor path, and pick the same OS
// fallback — they cannot diverge.
func (d *entryForm) openEditor() tea.Cmd {
	if !isTTY() {
		d.notice = "editor needs a TTY."

		return d.syncFormSize()
	}

	d.editorField = d.multilineFieldKey()

	buffer := d.value
	if d.editorField == fieldKeyDescription {
		buffer = d.description
	}

	tmp, err := os.CreateTemp("", "suve-tui-*.txt")
	if err != nil {
		d.notice = "could not open editor: " + err.Error()

		return d.syncFormSize()
	}

	name := tmp.Name()

	if _, err := tmp.WriteString(buffer); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		d.notice = "could not open editor: " + err.Error()

		return d.syncFormSize()
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

// onEditorFinished folds the editor buffer back into the field it was launched from
// (value/description): an unchanged buffer is a no-op ("No changes made."),
// otherwise the new content replaces that field and the form is rebuilt so the huh
// textarea buffer re-syncs (mirroring every other path).
func (d *entryForm) onEditorFinished(msg editorFinishedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		d.notice = "editor error: " + msg.err.Error()

		return d, d.syncFormSize()
	}

	// before holds the pre-edit buffer that was written to the tmpfile.
	before := &d.value
	if d.editorField == fieldKeyDescription {
		before = &d.description
	}

	// Most editors auto-append a trailing newline; normalize it away with the same
	// round-trip-lossless rule the CLI uses, so an untouched round-trip is
	// byte-identical to the seed and correctly reported as a no-op (instead of
	// silently mutating the field with a stray newline).
	content := editor.Normalize(*before, msg.content)

	if content == *before {
		d.notice = "No changes made."

		return d, d.syncFormSize()
	}

	*before = content
	d.notice = "Loaded from editor."

	// Rebuild so the huh textarea re-binds to the edited field (consistent with
	// every other value-changing path; avoids a stale textarea buffer).
	return d, d.rebuildForm()
}

func (d *entryForm) View() string {
	// The Stage/Apply popup replaces the whole form body while it is open (the app
	// shell frames and centers it as a compact box).
	if d.confirming {
		return d.confirm.View(d.styles)
	}

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

// header renders the dialog title, wrapped to the dialog width so a long edited
// entry name does not overflow the box (its height is budgeted into the form
// body so the whole dialog stays on-screen).
func (d *entryForm) header() string {
	return d.fit(d.styles.PaneTitle.Render(d.confirmTitle()))
}

// footer renders the pinned rows below the form: any active error and notice
// (each wrapped to the dialog width so a long provider error/notice stays inside
// the box), then the key hint. The error and notice are each capped to the rows
// that remain after the form's minimum body, so even a pathological error or
// notice scrolls the form rather than pushing the hint off the bottom.
func (d *entryForm) footer() string {
	parts := make([]string, 0, 3) //nolint:mnd // at most error + notice + hint

	// Wrap the hint to the dialog's fixed content width so the (now longer, #791)
	// key hint stays inside the box — folding to a second line at the minimum size
	// — instead of overflowing the border, and the box keeps the form's width.
	hint := d.styles.PageHint.Width(dialogContentWidth).Render(entryHint(d.multilineFieldKey() != ""))
	// Reserve the frame around the footer (title, spacer, the form's minimum body,
	// the hint); the error then the notice each take what remains, so the form
	// body never drops below minFormBody and the whole dialog fits.
	reserved := lipgloss.Height(d.header()) + titleSpacerRows + minFormBody + lipgloss.Height(hint)

	if d.err != "" {
		line := d.wrapCapped(d.styles.ErrorText.Render(d.err), d.errBudget(reserved))
		parts = append(parts, line)
		reserved += lipgloss.Height(line)
	}

	if d.notice != "" {
		line := d.wrapCapped(d.styles.Banner.Render(d.notice), d.errBudget(reserved))
		parts = append(parts, line)
	}

	parts = append(parts, hint)

	return strings.Join(parts, "\n")
}

// entryHint is the bottom hint line. On a multi-line field (Value/Description)
// Enter inserts a newline and ctrl+o hands off to $EDITOR; Tab moves between fields
// and the form is submitted from the "[ OK ]" button (Enter). On the single-line
// fields Enter also advances.
func entryHint(onMultiline bool) string {
	if onMultiline {
		return "tab/shift+tab: fields · enter: newline · ctrl+o: $EDITOR · esc: cancel"
	}

	return "tab/shift+tab: fields · enter: next · esc: cancel"
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

	// An immediate create that upserted onto an existing entry (o.Updated) reports
	// as an update, so the status matches what actually happened (create-or-update
	// parity with the GUI/CLI). Staged creates never upsert, so o.Updated is only
	// ever set on the immediate path.
	verb := "create"
	if edit || o.Updated {
		verb = "update"
	}

	if staged {
		return "Staged " + verb + "."
	}

	return "Applied " + verb + "."
}

// nameValidator builds the create name field's validator: the name is required,
// and it must not already be staged for deletion. Validating client-side against
// the delete-staged key set the browser already holds turns what would otherwise
// be a raw post-submit reducer error ("cannot add to delete-staged") into an
// inline, friendly message before the write is ever attempted (#692). The key is
// (typed name, current namespace) read from the live namespace pointer; for the
// common create (a concrete seeded namespace — an all-namespaces create is
// blocked upstream) the namespace is fixed, so the check is exact. Should a later
// namespace edit slip a delete-staged name past this, the reducer still refuses
// the write — this only upgrades the common case to a friendlier message.
func (d *entryForm) nameValidator() func(string) error {
	required := requiredField("name")

	return func(s string) error {
		if err := required(s); err != nil {
			return err
		}

		if _, ok := d.deleteStagedKeys[data.StagedKey{Name: s, Namespace: d.namespace}]; ok {
			return stringError(s + " is staged for deletion — reset it first")
		}

		return nil
	}
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
