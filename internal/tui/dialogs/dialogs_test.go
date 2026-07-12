//nolint:testpackage // white-box: drives the dialogs' Update/submit and inspects their unexported state
package dialogs

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	return capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: true}
}

func appConfigCap() capability.ServiceCapability {
	return capability.ServiceCapability{Service: "param", HasTags: true, HasStaging: true, HasNamespaces: true}
}

func awsSecretCap() capability.ServiceCapability {
	return capability.ServiceCapability{
		Service: "secret", HasTags: true, HasStaging: true, HasRestore: true,
		HasForceDelete: true, HasRecoveryWindow: true,
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

// TestEntryForm_TypeSelectGating pins the Type select is offered only for the
// typed AWS SSM param service (App Configuration is untyped; secret has none).
func TestEntryForm_TypeSelectGating(t *testing.T) {
	t.Parallel()

	awsParam, _ := newEntry(t, awsParamCap(), false)
	assert.True(t, awsParam.showType())

	appConfig, _ := newEntry(t, appConfigCap(), false)
	assert.False(t, appConfig.showType())

	secret, _ := newEntry(t, awsSecretCap(), false)
	assert.False(t, secret.showType())
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

// TestEntryForm_EditorNoOp pins the $EDITOR no-op: an unchanged buffer keeps the
// value and shows "No changes made."; a changed buffer replaces it.
func TestEntryForm_EditorNoOp(t *testing.T) {
	t.Parallel()

	d, _ := newEntry(t, awsParamCap(), true)
	d.value = "same"

	_, _ = d.onEditorFinished(editorFinishedMsg{content: "same"})
	assert.Equal(t, "same", d.value)
	assert.Equal(t, "No changes made.", d.notice)

	_, _ = d.onEditorFinished(editorFinishedMsg{content: "edited"})
	assert.Equal(t, "edited", d.value, "a changed buffer replaces the value")
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
