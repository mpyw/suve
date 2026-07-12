//nolint:testpackage // white-box: drives the dialogs' Update/submit and inspects their unexported state
package dialogs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// fakeMutator records the last routed write so a dialog's routing can be
// asserted without a real store.
type fakeMutator struct {
	svcCap capability.ServiceCapability

	createCalled  bool
	updateCalled  bool
	deleteCalled  bool
	addTagCalled  bool
	restoreCalled bool

	key            data.StagedKey
	value          string
	typeLabel      string
	staged         bool
	force          bool
	recoveryWindow int
	tagKey         string
	tagValue       string

	outcome data.WriteOutcome
	err     error
}

func (m *fakeMutator) Capability() capability.ServiceCapability { return m.svcCap }

func (m *fakeMutator) Create(_ context.Context, key data.StagedKey, value, typeLabel, _ string, staged bool) (data.WriteOutcome, error) {
	m.createCalled = true
	m.key, m.value, m.typeLabel, m.staged = key, value, typeLabel, staged

	return m.outcome, m.err
}

func (m *fakeMutator) Update(_ context.Context, key data.StagedKey, value, typeLabel, _ string, staged bool) (data.WriteOutcome, error) {
	m.updateCalled = true
	m.key, m.value, m.typeLabel, m.staged = key, value, typeLabel, staged

	return m.outcome, m.err
}

func (m *fakeMutator) Delete(_ context.Context, key data.StagedKey, force bool, recoveryWindow int, staged bool) (data.WriteOutcome, error) {
	m.deleteCalled = true
	m.key, m.force, m.recoveryWindow, m.staged = key, force, recoveryWindow, staged

	return m.outcome, m.err
}

func (m *fakeMutator) AddTag(_ context.Context, key data.StagedKey, tagKey, tagValue string, staged bool) (data.WriteOutcome, error) {
	m.addTagCalled = true
	m.key, m.tagKey, m.tagValue, m.staged = key, tagKey, tagValue, staged

	return m.outcome, m.err
}

func (m *fakeMutator) RemoveTag(_ context.Context, key data.StagedKey, tagKey string, staged bool) (data.WriteOutcome, error) {
	m.key, m.tagKey, m.staged = key, tagKey, staged

	return m.outcome, m.err
}

func (m *fakeMutator) Restore(context.Context, string) (data.WriteOutcome, error) {
	m.restoreCalled = true

	return m.outcome, m.err
}

// Capability fixtures.
func awsParamCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: true, HasDescription: true}
}

func appConfigCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: true, HasNamespaces: true}
}

func awsSecretCap() capability.ServiceCapability {
	return capability.ServiceCapability{
		Service: "secret", HasTags: true, HasStaging: true, HasRestore: true,
		HasForceDelete: true, HasRecoveryWindow: true, HasDescription: true,
	}
}

func gcloudSecretCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "secret", HasTags: true, HasStaging: true}
}

func noStagingParamCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: false}
}

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

// execCmd runs a command for its side effect on the recording mutator.
func execCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	require.NotNil(t, cmd)

	_ = cmd()
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

// TestEntryForm_StagedOnlyHidesModeToggle pins the #679 fix: a dialog launched
// from a staged-only surface (the staging review page) hides the Stage/Apply-
// immediately mode toggle and forces a staged write, so the review screen offers
// no immediate-write escape hatch that would bypass the staging store. A default
// (browser) launch still shows the toggle.
func TestEntryForm_StagedOnlyHidesModeToggle(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}

	browser, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(),
		Edit: true, Name: "/app/X", Value: "old",
	})
	b, ok := browser.(*entryForm)
	require.True(t, ok)
	assert.True(t, b.staged, "browser launch defaults to staged")
	assert.Contains(t, b.View(), "Apply immediately", "browser launch keeps the mode toggle")

	staged, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(),
		Edit: true, Name: "/app/X", Value: "old", StagedOnly: true,
	})
	s, ok := staged.(*entryForm)
	require.True(t, ok)
	assert.True(t, s.staged, "a staged-only launch forces staged")
	assert.NotContains(t, s.View(), "Apply immediately", "a staged-only launch hides the mode toggle")

	// The forced-staged write still routes through the staging path.
	execCmd(t, s.submit())
	assert.True(t, mut.staged, "a staged-only submit writes staged, never immediate")
}

// TestTagForm_StagedOnlyHidesModeToggle pins the #679 fix for the tag dialog: a
// staged-only launch hides the mode toggle and forces a staged tag write; a
// browser launch keeps the toggle.
func TestTagForm_StagedOnlyHidesModeToggle(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}

	browser, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
	})
	b, ok := browser.(*tagForm)
	require.True(t, ok)
	assert.True(t, b.staged, "browser launch defaults to staged")
	assert.Contains(t, b.View(), "Apply immediately", "browser launch keeps the mode toggle")

	staged, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(),
		Name: "/app/X", StagedOnly: true,
	})
	s, ok := staged.(*tagForm)
	require.True(t, ok)
	assert.True(t, s.staged, "a staged-only launch forces staged")
	assert.NotContains(t, s.View(), "Apply immediately", "a staged-only launch hides the mode toggle")

	s.tagKey = "owner"
	execCmd(t, s.submit())
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
	assert.Equal(t, "prod", namespaceDisplay("prod"))
	assert.Equal(t, "(default)", namespaceDisplay(""))
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
	assert.True(t, awsParam.showType(), "staged mode still offers the Type select")
	assert.Contains(t, awsParam.View(), "Type", "the staged form draws the Type row (default mode)")

	awsParam.staged = false
	require.NotNil(t, awsParam.rebuildForm())
	assert.True(t, awsParam.showType(), "immediate mode offers the Type select")
	assert.Contains(t, awsParam.View(), "Type", "immediate form draws the Type row")

	appConfig, _ := newEntry(t, appConfigCap(), false)
	assert.False(t, appConfig.showType(), "App Configuration is untyped in either mode")
	appConfig.staged = false
	require.NotNil(t, appConfig.rebuildForm())
	assert.False(t, appConfig.showType(), "App Configuration is untyped in either mode")

	secret, _ := newEntry(t, awsSecretCap(), false)
	assert.False(t, secret.showType(), "secret has no value type in either mode")
	secret.staged = false
	require.NotNil(t, secret.rebuildForm())
	assert.False(t, secret.showType(), "secret has no value type in either mode")

	// A staged-only surface (the staging review page's edit) hides the Type select:
	// the write is always a staged edit that preserves the existing type, and the
	// dialog cannot seed the entry's current type.
	stagedOnly := newStagedOnlyEntry(t, awsParamCap())
	assert.False(t, stagedOnly.showType(), "a staged-only edit hides the Type select")
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
// for a service that honors it (AWS param/secret). The gcloud, Azure Key Vault,
// and App Configuration writers ignore a description, so their forms omit it.
func TestEntryForm_DescriptionGating(t *testing.T) {
	t.Parallel()

	awsParam, _ := newEntry(t, awsParamCap(), false)
	assert.Contains(t, awsParam.View(), "Description", "AWS param offers a description")

	awsSecret, _ := newEntry(t, awsSecretCap(), false)
	assert.Contains(t, awsSecret.View(), "Description", "AWS secret offers a description")

	appConfig := newAppConfigEntry(t, false, "prod")
	assert.NotContains(t, appConfig.View(), "Description", "App Configuration ignores a description")

	gcloudSecret, _ := newEntry(t, gcloudSecretCap(), false)
	assert.NotContains(t, gcloudSecret.View(), "Description", "gcloud secret ignores a description")
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

	_, _ = d.onEditorFinished(editorFinishedMsg{content: string(raw)})
	assert.Equal(t, "same", d.value, "an editor-appended newline is a no-op round-trip")
	assert.Equal(t, "No changes made.", d.notice)

	// A genuine edit still replaces the value (with the editor newline normalized).
	require.NoError(t, os.WriteFile(tmp, []byte("edited\n"), 0o600))
	raw, err = os.ReadFile(tmp) //nolint:gosec // tmp is this test's own temp file
	require.NoError(t, err)

	_, _ = d.onEditorFinished(editorFinishedMsg{content: string(raw)})
	assert.Equal(t, "edited", d.value, "a changed buffer replaces the value (newline normalized)")
	assert.Equal(t, "Loaded from editor.", d.notice)
}

// TestEntryForm_EditorNoTTY pins the TTY gate: without a TTY the editor is not
// launched and a notice explains why.
func TestEntryForm_EditorNoTTY(t *testing.T) { //nolint:paralleltest // swaps the package isTTY seam
	orig := isTTY
	isTTY = func() bool { return false }

	t.Cleanup(func() { isTTY = orig })

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
}

// TestDeleteConfirm_ForceRecoveryGating pins that force/recovery rows appear only
// for a service with those capabilities, and are mutually exclusive (forcing
// hides the recovery row).
func TestDeleteConfirm_ForceRecoveryGating(t *testing.T) {
	t.Parallel()

	aws := newDelete(t, awsSecretCap())
	assert.Contains(t, aws.controls(), ctrlForce)
	assert.Contains(t, aws.controls(), ctrlRecovery)

	aws.force = true
	assert.NotContains(t, aws.controls(), ctrlRecovery, "forcing hides the recovery row (mutual exclusion)")
	assert.Equal(t, 0, aws.effectiveRecoveryWindow(), "a forced delete records no recovery window")

	gcloud := newDelete(t, gcloudSecretCap())
	assert.NotContains(t, gcloud.controls(), ctrlForce)
	assert.NotContains(t, gcloud.controls(), ctrlRecovery)
}

// TestDeleteConfirm_RecoveryStagedOnly pins that the recovery-window row (and its
// "recoverable until" line) shows only for a STAGED delete. An immediate delete
// cannot carry a custom window — the SDK-neutral seam has no recovery-window
// DeleteOption, so AWS applies its 30-day default — so the dialog must not offer
// an adjustable window it would silently drop (GUI parity: SecretDelete(name,
// force) exposes only force for immediate).
func TestDeleteConfirm_RecoveryStagedOnly(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	require.True(t, d.staged, "AWS secret delete defaults to staged")
	assert.Contains(t, d.controls(), ctrlRecovery, "staged mode offers the recovery window")
	assert.Contains(t, d.View(), "Recovery window", "staged mode draws the adjustable window")

	d.staged = false
	assert.NotContains(t, d.controls(), ctrlRecovery, "immediate mode hides the recovery window")
	assert.NotContains(t, d.View(), "Recovery window", "immediate mode shows no adjustable window")
	assert.NotContains(t, d.View(), "Recoverable until", "immediate mode makes no recoverable-until claim")
	assert.Equal(t, 0, d.effectiveRecoveryWindow(), "immediate mode records no custom window")
}

// TestDeleteConfirm_ModeToggleKeepsFocusAndDropsRecovery drives the mode toggle
// through activate() and pins that toggling to immediate keeps focus on the Mode
// row (rather than drifting onto a neighbour as the recovery row disappears) and
// removes the recovery control.
func TestDeleteConfirm_ModeToggleKeepsFocusAndDropsRecovery(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())

	// Focus the Mode row: [force, recovery, mode, delete, cancel] -> index 2.
	d.focus = 2
	require.Equal(t, ctrlMode, d.focused())

	_, _ = d.activate() // toggle staged -> immediate

	assert.False(t, d.staged, "activate toggles the mode to immediate")
	assert.Equal(t, ctrlMode, d.focused(), "focus stays on the Mode row after the control set shrinks")
	assert.NotContains(t, d.controls(), ctrlRecovery, "immediate mode drops the recovery row")
}

// TestDeleteConfirm_SubmitRouting pins the delete routing (force/window/staged).
func TestDeleteConfirm_SubmitRouting(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	d.name = "prod/key"
	d.staged = true

	execCmd(t, d.submit())

	got, ok := d.mutator.(*fakeMutator)
	require.True(t, ok)
	assert.True(t, got.deleteCalled)
	assert.True(t, got.staged)
	assert.Equal(t, defaultRecoveryWindow, got.recoveryWindow)
	assert.Equal(t, "prod/key", got.key.Name)
}

// TestDeleteConfirm_ModeGating pins that the mode toggle is present only when the
// service supports staging.
func TestDeleteConfirm_ModeGating(t *testing.T) {
	t.Parallel()

	staged := newDelete(t, awsSecretCap())
	assert.Contains(t, staged.controls(), ctrlMode)
	assert.True(t, staged.staged)

	noStaging := newDelete(t, capability.ServiceCapability{Service: "secret"})
	assert.NotContains(t, noStaging.controls(), ctrlMode)
	assert.False(t, noStaging.staged)
}

// TestDeleteConfirm_BusySuppression pins the double-submit guard.
func TestDeleteConfirm_BusySuppression(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	d.busy = true

	_, cmd := d.Update(keyMsg('\r'))
	assert.Nil(t, cmd, "input is swallowed while busy")
	assert.True(t, d.Busy())
}

// TestTagForm_Routing pins that the tag form routes to AddTag/RemoveTag per the
// action select and carries the mode.
func TestTagForm_Routing(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	d.tagKey, d.tagValue, d.staged = "owner", "team", true

	d.remove = false
	execCmd(t, d.submit())
	assert.True(t, mut.addTagCalled, "add action routes to AddTag")
	assert.True(t, mut.staged)

	d.remove = true
	execCmd(t, d.submit())
	assert.Equal(t, "owner", mut.tagKey, "remove action routes to RemoveTag with the key")
}

// TestDeleteConfirm_ResultVoicing pins the delete status voicing, including the
// auto-unstage case: deleting a staged create removes it (nothing left to
// delete). The pure voicing is asserted, then routed through onResult to confirm
// it reaches the MutationDoneMsg status line.
func TestDeleteConfirm_ResultVoicing(t *testing.T) {
	t.Parallel()

	assert.Contains(t, deleteStatus(true, data.WriteOutcome{Unstaged: true}), "nothing left to delete")
	assert.Equal(t, "Staged delete.", deleteStatus(true, data.WriteOutcome{}))
	assert.Equal(t, "Deleted.", deleteStatus(false, data.WriteOutcome{}))

	d := newDelete(t, awsSecretCap())
	d.staged = true

	_, cmd := d.onResult(mutationResultMsg{outcome: data.WriteOutcome{Unstaged: true}})
	done, ok := cmd().(MutationDoneMsg)
	require.True(t, ok, "a successful delete emits MutationDoneMsg")
	assert.Contains(t, done.Status, "nothing left to delete", "auto-unstage surfaces in the status line")
}

// TestTagForm_ResultVoicing pins the tag status voicing (staged/applied,
// add/removal) and that it reaches the MutationDoneMsg status line. Tags carry no
// auto-unstage/skip outcome today (TagOutput/UntagOutput hold only the name), so
// the tag equivalent that surfaces is the staged/applied voicing.
func TestTagForm_ResultVoicing(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Staged tag add.", tagStatus(false, true))
	assert.Equal(t, "Staged tag removal.", tagStatus(true, true))
	assert.Equal(t, "Applied tag add.", tagStatus(false, false))
	assert.Equal(t, "Applied tag removal.", tagStatus(true, false))

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	d.remove, d.staged = true, true

	_, cmd := d.onResult(mutationResultMsg{outcome: data.WriteOutcome{}})
	done, ok := cmd().(MutationDoneMsg)
	require.True(t, ok, "a successful tag write emits MutationDoneMsg")
	assert.Equal(t, "Staged tag removal.", done.Status, "tag voicing surfaces in the status line")
}

// TestRestoreForm_Routing pins that the restore form routes to Restore.
func TestRestoreForm_Routing(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsSecretCap()}
	m, _ := NewRestore(RestoreInput{
		Ctx: context.Background(), Mutator: mut, Service: "secret", Styles: styles.New(), Name: "prod/x",
	})
	d, ok := m.(*restoreForm)
	require.True(t, ok)

	execCmd(t, d.submit())
	assert.True(t, mut.restoreCalled)
}

func newDelete(t *testing.T, svcCap capability.ServiceCapability) *deleteConfirm {
	t.Helper()

	mut := &fakeMutator{svcCap: svcCap}
	m := NewDeleteConfirm(DeleteInput{
		Ctx: context.Background(), Mutator: mut, Service: svcCap.Service, Styles: styles.New(), Name: "x",
	})

	d, ok := m.(*deleteConfirm)
	require.True(t, ok)

	return d
}

// keyMsg builds a printable key press.
func keyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}
