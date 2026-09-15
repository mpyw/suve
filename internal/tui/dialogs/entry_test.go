//nolint:testpackage // white-box: drives the entry form's Update/submit and inspects its unexported state
package dialogs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

func newEntry(t *testing.T, svcCap capability.ServiceCapability, edit bool) (*entryForm, *fakeMutator) {
	t.Helper()

	mut := &fakeMutator{svcCap: svcCap}

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: svcCap.Service, Styles: styles.New(), Edit: edit,
		Name: "/app/X", Value: "old", TypeLabel: "SecureString",
	})

	d, ok := m.(*entryForm)
	require.True(t, ok)

	return d, mut
}

// newStagedOnlyEntry builds a staged-only edit form (the staging review page's
// edit path): the mode toggle is hidden and the write is forced staged.
func newStagedOnlyEntry(t *testing.T, svcCap capability.ServiceCapability) *entryForm {
	t.Helper()

	mut := &fakeMutator{svcCap: svcCap}

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: svcCap.Service, Styles: styles.New(),
		Edit: true, Name: "/app/X", Value: "old", StagedOnly: true,
	})

	d, ok := m.(*entryForm)
	require.True(t, ok)

	return d
}

// TestEntryForm_StagedByDefault pins that the mode defaults to Stage when the
// service supports staging, and is forced immediate (no toggle) otherwise.
func TestEntryForm_StagedByDefault(t *testing.T) {
	t.Parallel()

	staged, _ := newEntry(t, awsParamCap(), false)
	assert.True(t, staged.staged, "staged is the default when the service supports staging")

	immediate, _ := newEntry(t, noStagingParamCap(), false)
	assert.False(t, immediate.staged, "without staging the write is always immediate")
}

// TestEntryForm_StagedOnlySkipsConfirm pins the #679 fix: a dialog launched from a
// staged-only surface (the staging review page) offers no Stage/Apply choice and
// forces a staged write, so the review screen has no immediate-write escape hatch
// that would bypass the staging store. A default (browser) launch instead opens the
// Stage/Apply popup on submit.
func TestEntryForm_StagedOnlySkipsConfirm(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}

	browser, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(),
		Edit: true, Name: "/app/X", Value: "old",
	})
	b, ok := browser.(*entryForm)
	require.True(t, ok)
	assert.True(t, b.staged, "browser launch defaults to staged")

	m, _ := b.beginSubmit()
	b, ok = m.(*entryForm)
	require.True(t, ok)
	assert.True(t, b.confirming, "a browser submit opens the Stage/Apply popup")
	assert.Contains(t, b.View(), "Apply immediately", "the popup offers the mode choice")

	staged, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(),
		Edit: true, Name: "/app/X", Value: "old", StagedOnly: true,
	})
	s, ok := staged.(*entryForm)
	require.True(t, ok)
	assert.True(t, s.staged, "a staged-only launch forces staged")

	m, cmd := s.beginSubmit()
	s, ok = m.(*entryForm)
	require.True(t, ok)
	assert.False(t, s.confirming, "a staged-only submit shows no popup")
	require.NotNil(t, cmd, "a staged-only submit writes directly")

	execCmd(t, cmd)
	assert.True(t, mut.staged, "a staged-only submit writes staged, never immediate")
}

// TestEntryForm_CreateNameRejectsDeleteStaged pins the create-name client-side
// validation half of #692: the name field's validator rejects a name that is
// already staged for deletion with an inline friendly message, so the write never
// reaches the reducer's raw post-submit "cannot add to delete-staged" error. The
// key is (name, namespace), so a same-name entry under a different namespace does
// not collide, and a required-name error still fires for an empty name.
func TestEntryForm_CreateNameRejectsDeleteStaged(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(),
		DeleteStagedKeys: map[data.StagedKey]struct{}{{Name: "/app/doomed"}: {}},
	})
	d, ok := m.(*entryForm)
	require.True(t, ok)

	validate := d.nameValidator()

	err := validate("/app/doomed")
	require.Error(t, err, "a delete-staged name is rejected client-side")
	assert.Contains(t, err.Error(), "staged for deletion", "the message names the reason")

	require.NoError(t, validate("/app/fresh"), "a name that is not delete-staged is accepted")
	require.Error(t, validate(""), "the required-name check still fires")

	// The key is (name, namespace): the same name under a different namespace is
	// not the delete-staged (empty-namespace) key, so it is accepted.
	d.namespace = "other"

	require.NoError(t, validate("/app/doomed"), "a same-name entry under a different namespace does not collide")
}

// newAppConfigEntry builds an App Configuration (namespaced) create/edit form
// seeded with a namespace, for the namespace read-only assertions.
func newAppConfigEntry(t *testing.T, edit bool, namespace string) *entryForm {
	t.Helper()

	mut := &fakeMutator{svcCap: appConfigCap()}

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Edit: edit,
		Name: "app/Feature", Namespace: namespace, Value: "old",
	})

	d, ok := m.(*entryForm)
	require.True(t, ok)

	return d
}

// TestEntryForm_NamespaceReadOnlyOnEdit pins that the App Configuration namespace
// field is an editable input on CREATE but a read-only note on EDIT: a write
// targets one concrete namespace, so editing the namespace of an existing entry
// would silently retarget a different partition (the name field is likewise
// omitted on edit).
func TestEntryForm_NamespaceReadOnlyOnEdit(t *testing.T) {
	t.Parallel()

	create := newAppConfigEntry(t, false, "prod")
	_, isInput := create.namespaceField().(*huh.Input)
	assert.True(t, isInput, "create offers an editable namespace input")

	edit := newAppConfigEntry(t, true, "prod")
	_, isNote := edit.namespaceField().(*huh.Note)
	assert.True(t, isNote, "edit renders the namespace read-only (a note, not an input)")

	// The null (default) namespace is shown as "(default)" rather than a blank.
	assert.Equal(t, "prod", entryNamespaceDisplay("prod"))
	assert.Equal(t, "(default)", entryNamespaceDisplay(""))
}

// TestEntryForm_TypeSelectGating pins the Type select is offered for the typed
// AWS SSM param service (App Configuration is untyped; secret has none) in BOTH
// modes: the value type flows through the staged path as well as the immediate
// path (the #664 fix), so the select is reachable regardless of the mode toggle
// — where it was previously hidden in staged mode as a #664 containment. It must
// stay absent for the untyped services.
func TestEntryForm_TypeSelectGating(t *testing.T) {
	t.Parallel()

	awsParam, _ := newEntry(t, awsParamCap(), false)
	require.True(t, awsParam.staged, "AWS param defaults to staged")
	assert.True(t, awsParam.offersType(), "staged mode still offers the Type select")
	assert.Contains(t, awsParam.View(), "Type", "the staged form draws the Type row (default mode)")

	awsParam.staged = false
	require.NotNil(t, awsParam.rebuildForm())
	assert.True(t, awsParam.offersType(), "immediate mode offers the Type select")
	assert.Contains(t, awsParam.View(), "Type", "immediate form draws the Type row")

	appConfig, _ := newEntry(t, appConfigCap(), false)
	assert.False(t, appConfig.offersType(), "App Configuration is untyped in either mode")
	appConfig.staged = false
	require.NotNil(t, appConfig.rebuildForm())
	assert.False(t, appConfig.offersType(), "App Configuration is untyped in either mode")

	secret, _ := newEntry(t, awsSecretCap(), false)
	assert.False(t, secret.offersType(), "secret has no value type in either mode")
	secret.staged = false
	require.NotNil(t, secret.rebuildForm())
	assert.False(t, secret.offersType(), "secret has no value type in either mode")

	// A staged-only surface (the staging review page's edit) hides the Type select:
	// the write is always a staged edit that preserves the existing type, and the
	// dialog cannot seed the entry's current type.
	stagedOnly := newStagedOnlyEntry(t, awsParamCap())
	assert.False(t, stagedOnly.offersType(), "a staged-only edit hides the Type select")
	assert.NotContains(t, stagedOnly.View(), "Type", "a staged-only edit draws no Type row")
}

// TestEntryForm_StagedCarriesType pins that a staged param create routes the
// selected value type (e.g. SecureString) through to the mutator — the TUI half
// of the #664/#680 fix. Previously the staged path dropped the type, silently
// creating the parameter as plaintext String.
func TestEntryForm_StagedCarriesType(t *testing.T) {
	t.Parallel()

	d, mut := newEntry(t, awsParamCap(), false)
	d.name = "/app/SECRET"
	d.value = "s3cr3t"
	d.valueType = "SecureString"
	d.staged = true

	execCmd(t, d.submit())

	assert.True(t, mut.createCalled)
	assert.True(t, mut.staged, "the write is staged")
	assert.Equal(t, "SecureString", mut.typeLabel, "the staged create carries the selected type")
}

// TestEntryForm_StagedOnlyEditPreservesType pins that a staged-only edit (the
// staging review page's edit, which shows no Type control and cannot seed the
// entry's current type) submits an EMPTY type label. An empty label preserves the
// existing staged/cloud type in the staging apply, so re-editing a staged
// SecureString from the review page never silently downgrades it to plaintext —
// the failure this would otherwise reintroduce once the Type select is reachable.
func TestEntryForm_StagedOnlyEditPreservesType(t *testing.T) {
	t.Parallel()

	d := newStagedOnlyEntry(t, awsParamCap())
	d.value = "new"

	mut, ok := d.mutator.(*fakeMutator)
	require.True(t, ok)

	execCmd(t, d.submit())

	assert.True(t, mut.updateCalled)
	assert.True(t, mut.staged, "a staged-only edit is staged")
	assert.Empty(t, mut.typeLabel, "a staged-only edit passes no type, so the existing type is preserved")
}

// TestEntryForm_DescriptionGating pins that the Description field is drawn only
// for a service that honors it: AWS param/secret (native) and Google Cloud
// secret (stored as the "description" annotation). The Azure Key Vault and App
// Configuration writers have no description concept, so their forms omit it.
func TestEntryForm_DescriptionGating(t *testing.T) {
	t.Parallel()

	awsParam, _ := newEntry(t, awsParamCap(), false)
	assert.Contains(t, awsParam.View(), "Description", "AWS param offers a description")

	awsSecret, _ := newEntry(t, awsSecretCap(), false)
	assert.Contains(t, awsSecret.View(), "Description", "AWS secret offers a description")

	gcloudSecret, _ := newEntry(t, gcloudSecretCap(), false)
	assert.Contains(t, gcloudSecret.View(), "Description", "gcloud secret offers a description (annotation-backed)")

	appConfig := newAppConfigEntry(t, false, "prod")
	assert.NotContains(t, appConfig.View(), "Description", "App Configuration has no description concept")
}

// TestEntryForm_SubmitRoutesStaged pins that a staged submit routes through the
// staging path (Create for a new entry, Update for an edit) with the key/value.
func TestEntryForm_SubmitRoutesStaged(t *testing.T) {
	t.Parallel()

	d, mut := newEntry(t, awsParamCap(), false)
	d.name = "/app/NEW"
	d.value = "v1"
	d.valueType = "String"
	d.staged = true

	execCmd(t, d.submit())

	assert.True(t, mut.createCalled)
	assert.False(t, mut.updateCalled)
	assert.True(t, mut.staged)
	assert.Equal(t, data.StagedKey{Name: "/app/NEW"}, mut.key)
	assert.Equal(t, "v1", mut.value)
}

// TestEntryForm_GCloudDescriptionThreaded pins that a Google Cloud secret submit
// carries the Description through to the mutator (annotation-backed), for both a
// staged create and an immediate edit — the write axis of #666's gcloud support.
func TestEntryForm_GCloudDescriptionThreaded(t *testing.T) {
	t.Parallel()

	create, createMut := newEntry(t, gcloudSecretCap(), false)
	create.name = "my-secret"
	create.value = "v1"
	create.description = "app credentials"
	create.staged = true

	execCmd(t, create.submit())

	assert.True(t, createMut.createCalled)
	assert.Equal(t, "app credentials", createMut.description, "staged create threads the description")

	edit, editMut := newEntry(t, gcloudSecretCap(), true)
	edit.value = "v2"
	edit.description = "rotated key"
	edit.staged = false

	execCmd(t, edit.submit())

	assert.True(t, editMut.updateCalled)
	assert.Equal(t, "rotated key", editMut.description, "immediate edit threads the description")
}

// TestEntryForm_EditSubmitImmediate pins an edit dialog in immediate mode routes
// to Update with staged=false and preserves the type label.
func TestEntryForm_EditSubmitImmediate(t *testing.T) {
	t.Parallel()

	d, mut := newEntry(t, awsParamCap(), true)
	d.value = "new"
	d.staged = false

	execCmd(t, d.submit())

	assert.True(t, mut.updateCalled)
	assert.False(t, mut.createCalled)
	assert.False(t, mut.staged)
	assert.Equal(t, "SecureString", mut.typeLabel, "edit preserves the current type")
}

// TestEntryForm_EditorNoOp pins the editor no-op as a REAL round-trip: the value
// is written to a temp file, a simulated editor appends a trailing newline (as
// most editors do), and the file is read back through the actual read path
// (onEditorFinished). The newline-normalization must treat this as untouched
// ("No changes made.", value unchanged) rather than silently mutating the value
// with a stray newline; a genuine edit still replaces it.
func TestEntryForm_EditorNoOp(t *testing.T) {
	t.Parallel()

	d, _ := newEntry(t, awsParamCap(), true)
	d.value = "same"

	tmp := filepath.Join(t.TempDir(), "edit.txt")

	// Simulate an editor that saved the buffer untouched but appended a newline.
	require.NoError(t, os.WriteFile(tmp, []byte(d.value+"\n"), 0o600))
	raw, err := os.ReadFile(tmp) //nolint:gosec // tmp is this test's own temp file
	require.NoError(t, err)

	_, _ = d.onEditorFinished(entryEditorFinishedMsg{content: string(raw)})
	assert.Equal(t, "same", d.value, "an editor-appended newline is a no-op round-trip")
	assert.Equal(t, "No changes made.", d.notice)

	// A genuine edit still replaces the value (with the editor newline normalized).
	require.NoError(t, os.WriteFile(tmp, []byte("edited\n"), 0o600))
	raw, err = os.ReadFile(tmp) //nolint:gosec // tmp is this test's own temp file
	require.NoError(t, err)

	_, _ = d.onEditorFinished(entryEditorFinishedMsg{content: string(raw)})
	assert.Equal(t, "edited", d.value, "a changed buffer replaces the value (newline normalized)")
	assert.Equal(t, "Loaded from editor.", d.notice)
}

// TestEntryForm_EditorNoTTY pins the TTY gate: without a TTY the editor is not
// launched and a notice explains why.
func TestEntryForm_EditorNoTTY(t *testing.T) { //nolint:paralleltest // swaps the package entryIsTTY seam
	orig := entryIsTTY
	entryIsTTY = func() bool { return false }

	t.Cleanup(func() { entryIsTTY = orig })

	d, _ := newEntry(t, awsParamCap(), true)
	cmd := d.openEditor()

	assert.Nil(t, cmd, "no editor process is launched without a TTY")
	assert.Contains(t, d.notice, "TTY")
}

// TestEntryForm_BusySuppression pins the busy guard: while a mutation is in
// flight the dialog swallows input (no double-submit) and reports Busy(), and a
// result clears it.
func TestEntryForm_BusySuppression(t *testing.T) {
	t.Parallel()

	d, mut := newEntry(t, awsParamCap(), false)
	d.busy = true

	assert.True(t, d.Busy())

	_, cmd := d.Update(keyMsg('a'))
	assert.Nil(t, cmd, "input is swallowed while busy")
	assert.False(t, mut.createCalled, "no second submit while busy")

	_, _ = d.Update(mutationResultMsg{outcome: data.WriteOutcome{}})
	assert.False(t, d.Busy(), "a result clears the busy state")
}

// TestEntryForm_ResultVoicing pins the skip/unstage/staged status voicing.
func TestEntryForm_ResultVoicing(t *testing.T) {
	t.Parallel()

	assert.Contains(t, entryStatus(true, true, data.WriteOutcome{Skipped: true}), "nothing staged")
	assert.Contains(t, entryStatus(true, true, data.WriteOutcome{Unstaged: true}), "auto-unstaged")
	assert.Equal(t, "Staged update.", entryStatus(true, true, data.WriteOutcome{}))
	assert.Equal(t, "Applied create.", entryStatus(false, false, data.WriteOutcome{}))
	// #691: an immediate create that upserted onto an existing entry voices an
	// update, matching the GUI/CLI create-or-update semantics.
	assert.Equal(t, "Applied update.", entryStatus(false, false, data.WriteOutcome{Updated: true}))
}

// TestEntryForm_ImmediateCreateUpsertVoicesUpdate pins the #691 fix at the dialog
// layer: when an immediate param create upserts onto an existing entry, the
// mutator reports WriteOutcome{Updated: true}; the dialog must voice "Applied
// update." (never surface the raw already-exists error) and emit MutationDoneMsg.
func TestEntryForm_ImmediateCreateUpsertVoicesUpdate(t *testing.T) {
	t.Parallel()

	d, mut := newEntry(t, awsParamCap(), false)
	d.name = "/app/EXISTS"
	d.value = "new"
	d.staged = false
	// The data layer upserted onto an existing param: create fell back to update.
	mut.outcome = data.WriteOutcome{Updated: true}

	// Run the create submit and feed its result back through Update (the real loop).
	cmd := d.submit()
	require.NotNil(t, cmd)

	m, done := d.Update(cmd())

	dd, ok := m.(*entryForm)
	require.True(t, ok)
	assert.True(t, mut.createCalled, "an immediate create routes through Create")
	assert.False(t, mut.staged, "the write is immediate")
	assert.Empty(t, dd.err, "no raw already-exists error is surfaced")

	msg, ok := done().(MutationDoneMsg)
	require.True(t, ok, "a successful upsert emits MutationDoneMsg")
	assert.Equal(t, "Applied update.", msg.Status, "an upsert voices an update, not a create")
}

// TestEntryForm_EscDiscardGuard pins the #790 double-Esc guard: a clean form
// closes on the first Esc; a dirty form arms a confirmation on the first Esc
// (stays open, shows the notice) and discards only on the second consecutive Esc;
// any keystroke between the two Escs re-arms so a later single Esc is safe.
func TestEntryForm_EscDiscardGuard(t *testing.T) {
	t.Parallel()

	// Clean create form: the first Esc cancels immediately.
	clean := newCreateEntry(t, awsSecretCap())
	_, cmd := clean.Update(keyEsc())
	assert.True(t, isCanceled(cmd), "esc on a clean form cancels immediately")
	assert.False(t, clean.armed)

	// Dirty form: a typed value diverges from the empty seed.
	dirty := newCreateEntry(t, awsSecretCap())
	dirty.value = "half-typed value"

	// First Esc arms — no cancel, notice shown, stays open.
	_, cmd = dirty.Update(keyEsc())
	assert.True(t, dirty.armed, "first esc on a dirty form arms the discard")
	assert.Equal(t, discardNotice, dirty.notice)
	assert.False(t, isCanceled(cmd), "first esc on a dirty form does not cancel")

	// Second consecutive Esc discards.
	_, cmd = dirty.Update(keyEsc())
	assert.True(t, isCanceled(cmd), "a second consecutive esc discards")
}

// TestEntryForm_EscArmResetsOnKeystroke pins that any key between the two Escs
// resets the armed state, so a stray Esc after typing again does not discard.
func TestEntryForm_EscArmResetsOnKeystroke(t *testing.T) {
	t.Parallel()

	d := newCreateEntry(t, awsSecretCap())
	d.value = "half-typed value"

	_, _ = d.Update(keyEsc())
	require.True(t, d.armed)

	// A keystroke resets the armed state (and clears the discard notice).
	_, _ = d.Update(keyMsg('x'))
	assert.False(t, d.armed, "a keystroke resets the armed state")
	assert.Empty(t, d.notice)

	// So the next single Esc only re-arms, it does not discard.
	_, cmd := d.Update(keyEsc())
	assert.True(t, d.armed, "a later single esc re-arms")
	assert.False(t, isCanceled(cmd), "the re-armed first esc does not discard")
}

// TestEntryForm_ValueEnterInsertsNewline pins the #791 core: Enter in the Value
// textarea inserts a newline (does not submit or advance). An edit form focuses
// the Value field first, so a single Enter lands there.
func TestEntryForm_ValueEnterInsertsNewline(t *testing.T) {
	t.Parallel()

	d, _ := newEntry(t, awsSecretCap(), true) // edit: value is the first field
	m, _ := d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	d, ok := m.(*entryForm)
	require.True(t, ok)
	require.Equal(t, "value", focusedEntryFieldKey(d), "the edit form focuses the Value field first")

	m, _ = d.Update(keyEnter())
	d, ok = m.(*entryForm)
	require.True(t, ok)

	assert.Contains(t, d.value, "\n", "Enter inserts a newline in the Value textarea")
	assert.False(t, d.busy, "Enter in Value does not submit")
}

// TestEntryForm_ValueReadlineMotions pins the entry-form key fix at the layer the
// bug lived: driving the real huh form + bubbles textarea, Ctrl+A and Ctrl+E are
// readline start/end-of-line motions owned by the textarea — Ctrl+E is NOT huh's
// built-in "open editor" binding (which would launch its default nano). From the
// seeded "old" value, Ctrl+A then "X" prepends and Ctrl+E then "Y" appends,
// yielding "XoldY"; Ctrl+E also leaves the form editable rather than busy on an
// editor handoff. Before the fix, Ctrl+E did not move the caret (it opened the
// editor), so "Y" landed after "X" as "XYold" — a clean regression signal.
func TestEntryForm_ValueReadlineMotions(t *testing.T) {
	t.Parallel()

	d, _ := newEntry(t, awsSecretCap(), true) // edit: Value ("old") is the first field
	m, _ := d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	d, ok := m.(*entryForm)
	require.True(t, ok)
	require.Equal(t, "value", focusedEntryFieldKey(d), "the edit form focuses the Value field first")
	require.Equal(t, "old", d.value)

	// Ctrl+A → line start, prepend "X"; Ctrl+E → line end, append "Y".
	for _, msg := range []tea.KeyPressMsg{
		{Code: 'a', Mod: tea.ModCtrl},
		keyMsg('X'),
		{Code: 'e', Mod: tea.ModCtrl},
		keyMsg('Y'),
	} {
		m, _ = d.Update(msg)
		d, ok = m.(*entryForm)
		require.True(t, ok)
	}

	assert.Equal(t, "XoldY", d.value,
		"Ctrl+A moved to line start and Ctrl+E to line end (not huh's editor)")
	assert.False(t, d.busy, "Ctrl+E did not hand off to an external editor")
}

// TestEntryForm_SingleLineEnterAdvances pins that Enter on a single-line field
// (name) neither inserts a newline nor submits — it stays huh's advance key.
func TestEntryForm_SingleLineEnterAdvances(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsSecretCap()}
	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: "secret", Styles: styles.New(), Name: "the-name",
	})
	d, ok := m.(*entryForm)
	require.True(t, ok)
	require.Equal(t, "name", focusedEntryFieldKey(d), "the create form focuses the Name field first")

	m, _ = d.Update(keyEnter())
	d, ok = m.(*entryForm)
	require.True(t, ok)

	assert.NotContains(t, d.name, "\n", "Enter on a single-line field inserts no newline")
	assert.False(t, d.busy, "Enter on a single-line field does not submit")
	assert.False(t, mut.createCalled, "Enter on a single-line field does not write")
}

// TestEntryForm_ConfirmCommitStaged pins that completing the form opens the
// Stage/Apply popup (rather than writing directly), and Enter there commits with
// the default (staged) mode. The full ctrl+s → advance → complete flow through the
// live huh form is covered by the teatest interaction test in the tui package; here
// the completion path is driven directly via beginSubmit.
func TestEntryForm_ConfirmCommitStaged(t *testing.T) {
	t.Parallel()

	d, mut := newEntry(t, awsSecretCap(), true) // edit
	d.value = "some value"

	m, _ := d.beginSubmit()
	d, ok := m.(*entryForm)
	require.True(t, ok)

	require.True(t, d.confirming, "completing the form opens the Stage/Apply confirmation")
	assert.False(t, d.busy, "opening the popup does not yet write")
	assert.Contains(t, d.View(), "Apply immediately", "the popup offers the mode choice")

	m, cmd := d.Update(keyEnter())
	d, ok = m.(*entryForm)
	require.True(t, ok)

	require.NotNil(t, cmd, "enter in the popup emits the mutation command")
	assert.True(t, d.busy, "committing sets the busy guard")

	_ = cmd() // run the mutation against the recording mutator

	assert.True(t, mut.updateCalled, "committing routes through the mutator")
	assert.True(t, mut.staged, "the default choice stages the write")
}

// TestEntryForm_ConfirmApplyImmediately pins that choosing Apply immediately in
// the popup (→) writes immediately rather than staged.
func TestEntryForm_ConfirmApplyImmediately(t *testing.T) {
	t.Parallel()

	d, mut := newEntry(t, awsSecretCap(), true)
	d.value = "some value"

	m, _ := d.beginSubmit()
	d, _ = m.(*entryForm)
	m, _ = d.Update(keyRight()) // move selection to Apply immediately
	d, _ = m.(*entryForm)

	_, cmd := d.Update(keyEnter())
	require.NotNil(t, cmd)

	_ = cmd()

	assert.True(t, mut.updateCalled, "committing routes through the mutator")
	assert.False(t, mut.staged, "Apply immediately writes without staging")
}

// TestEntryForm_ConfirmBackReturnsToForm pins that esc in the Stage/Apply popup
// returns to the editable form without writing.
func TestEntryForm_ConfirmBackReturnsToForm(t *testing.T) {
	t.Parallel()

	d, mut := newEntry(t, awsSecretCap(), true)
	d.value = "some value"

	m, _ := d.beginSubmit()
	d, _ = m.(*entryForm)
	require.True(t, d.confirming)

	m, _ = d.Update(keyEsc())
	d, ok := m.(*entryForm)
	require.True(t, ok)

	assert.False(t, d.confirming, "esc dismisses the popup")
	assert.False(t, d.busy, "esc does not write")
	assert.False(t, mut.updateCalled, "esc attempts no mutation")
	assert.Contains(t, d.View(), "Value", "the editable form is shown again")
}

// TestEntryForm_NameValidatorRequired pins that the create name validator (which
// huh runs inline as the form advances) rejects an empty name, so an empty required
// name can never complete the form and reach the write.
func TestEntryForm_NameValidatorRequired(t *testing.T) {
	t.Parallel()

	d := newCreateEntry(t, awsSecretCap())

	require.Error(t, d.nameValidator()(""), "an empty name is rejected")
	require.ErrorContains(t, d.nameValidator()(""), "name is required")
	require.NoError(t, d.nameValidator()("/app/X"), "a non-empty name passes")
}

// TestEntryFormTheme_OKButtonInvertsOnFocus pins that the "[ OK ]" button is
// reverse-video only while focused: huh's single-affirmative Confirm always renders
// the affirmative with the group's FocusedButton, so the focus cue must live in the
// focused-vs-blurred button styles. Reverse (no hard-coded colors) reads in both
// light and dark terminals.
func TestEntryFormTheme_OKButtonInvertsOnFocus(t *testing.T) {
	t.Parallel()

	for _, dark := range []bool{true, false} {
		s := entryFormTheme().Theme(dark)
		assert.True(t, s.Focused.FocusedButton.GetReverse(), "the focused OK button inverts (dark=%v)", dark)
		assert.False(t, s.Blurred.FocusedButton.GetReverse(), "the blurred OK button is plain (dark=%v)", dark)
	}
}

// TestFormKeyMap_MultilineBindings pins the multi-line field key contract: Enter
// inserts a newline (never next/submit), Tab advances to the next field, and the
// field never submits on its own — the form is completed from the "[ OK ]" button,
// so a multi-line field is never the last field and its Submit is disabled.
func TestFormKeyMap_MultilineBindings(t *testing.T) {
	t.Parallel()

	km := entryFormKeyMap()
	tab := tea.KeyPressMsg{Code: tea.KeyTab}

	ctrlJ := tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
	ctrlE := tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}

	assert.True(t, key.Matches(keyEnter(), km.Text.NewLine), "enter inserts a newline")
	assert.True(t, key.Matches(ctrlJ, km.Text.NewLine), "ctrl+j inserts a newline")
	assert.True(t, key.Matches(tab, km.Text.Next), "tab advances to the next field")
	assert.False(t, key.Matches(keyEnter(), km.Text.Next), "enter does not advance (it inserts a newline)")
	assert.False(t, key.Matches(keyEnter(), km.Text.Submit), "a multi-line field never submits on enter")
	// ctrl+e must NOT trigger huh's built-in editor: the binding is disabled so the
	// key falls through to the textarea as readline end-of-line (not launch nano).
	assert.False(t, key.Matches(ctrlE, km.Text.Editor), "ctrl+e does not open huh's built-in editor")
}

// focusedEntryFieldKey reports the huh key of the currently focused field.
func focusedEntryFieldKey(d *entryForm) string {
	f := d.form.GetFocusedField()
	if f == nil {
		return ""
	}

	return f.GetKey()
}

// newCreateEntry builds a create form with no seeded fields, so it starts clean
// (every field empty) for the discard-guard and validation tests.
func newCreateEntry(t *testing.T, svcCap capability.ServiceCapability) *entryForm {
	t.Helper()

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: svcCap}, Service: svcCap.Service, Styles: styles.New(),
	})

	d, ok := m.(*entryForm)
	require.True(t, ok)

	return d
}

// TestEntryForm_TallFormFitsMinSize pins the #686 fix: at the minimum supported
// 60×16 terminal the create form — the tallest dialog — no longer clips its
// controls off the bottom. huh caps its body into a scrollable region so the
// whole box (including the submit/cancel hint) fits within the terminal height,
// and no line overflows the dialog width.
func TestEntryForm_TallFormFitsMinSize(t *testing.T) {
	t.Parallel()

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(),
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	view := m.View()

	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"the dialog body fits inside the frame, so the shell never clips it")
	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"no line overflows the dialog width at the minimum size")
	// The hint may fold across two lines at the minimum size, so match wrap-safe
	// affordance tokens rather than a phrase that could straddle the break.
	assert.Contains(t, flatten(view), "fields", "the field-nav hint stays on-screen")
	assert.Contains(t, flatten(view), "cancel", "the cancel hint stays on-screen")
}

// TestEntryForm_CompressesWhenTallerThanScreen pins that the min-size form is
// actually compressed (its body scrolled), not merely short: the same form at a
// tall terminal renders more rows and shows the full form down to the Description
// field, part of which scrolls out of view at 60×16.
func TestEntryForm_CompressesWhenTallerThanScreen(t *testing.T) {
	t.Parallel()

	build := func() Model {
		// The AWS param form is the tallest (Name + Type select with three options +
		// Value textarea + Description), so its body must scroll at the minimum size.
		m, _ := NewEntryForm(EntryFormInput{
			Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsParamCap()},
			Service: "param", Styles: styles.New(),
		})

		return m
	}

	small := build()
	small, _ = small.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	tall := build()
	tall, _ = tall.Update(tea.WindowSizeMsg{Width: minWidth, Height: 40})

	assert.Less(t, lipgloss.Height(small.View()), lipgloss.Height(tall.View()),
		"the min-size form is compressed relative to a tall terminal")
	assert.Contains(t, stripANSI(tall.View()), "Description",
		"a tall terminal shows the full form, down to the Description field")
}

// TestEntryForm_LongErrorStaysBounded pins that a long provider error after a
// failed create/edit keeps the huh-form dialog within the minimum size: the
// wrapped error is capped so the form body keeps at least minFormBody rows (huh
// scrolls it) and the submit/cancel hint stays on-screen instead of being pushed
// off the bottom. Driven through the real result path (onResult → rebuildForm →
// syncFormSize), which is how the error actually arrives.
func TestEntryForm_LongErrorStaysBounded(t *testing.T) {
	t.Parallel()

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(),
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	longErr := stringError(strings.Repeat("the create request was rejected by the provider and cannot be retried. ", 10))
	m, _ = m.Update(mutationResultMsg{err: longErr})

	view := m.View()
	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"a long error keeps the whole form within the terminal height")
	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"the wrapped error never overflows the dialog width")
	assert.Contains(t, flatten(view), "cancel", "the submit/cancel hint stays on-screen under a long error")
}
