//nolint:testpackage // white-box: drives the delete confirm's Update/submit and inspects its unexported state
package dialogs

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// TestDeleteConfirm_ForceRecoveryGating pins that force/recovery rows appear only
// for a service with those capabilities, and are mutually exclusive (forcing
// hides the recovery row).
func TestDeleteConfirm_ForceRecoveryGating(t *testing.T) {
	t.Parallel()

	aws := newDelete(t, awsSecretCap())
	assert.Contains(t, aws.controls(), deleteCtrlForce)
	assert.Contains(t, aws.controls(), deleteCtrlRecovery)

	aws.force = true
	assert.NotContains(t, aws.controls(), deleteCtrlRecovery, "forcing hides the recovery row (mutual exclusion)")
	assert.Equal(t, 0, aws.effectiveRecoveryWindow(), "a forced delete records no recovery window")

	gcloud := newDelete(t, gcloudSecretCap())
	assert.NotContains(t, gcloud.controls(), deleteCtrlForce)
	assert.NotContains(t, gcloud.controls(), deleteCtrlRecovery)
}

// TestDeleteConfirm_RecoveryVisibleAppliesStagedOnly pins that the recovery-window
// row shows whenever the service has a recovery window and force is off — the
// Stage/Apply choice moved to the popup, so the row no longer depends on it — while
// its VALUE still applies only to a staged delete: an immediate delete cannot carry
// a custom window (the SDK-neutral seam has no recovery-window DeleteOption, so AWS
// applies its 30-day default), so effectiveRecoveryWindow is 0 when immediate is
// chosen. Forcing hides the row (mutual exclusion).
func TestDeleteConfirm_RecoveryVisibleAppliesStagedOnly(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	require.True(t, d.staged, "AWS secret delete defaults to staged")
	assert.Contains(t, d.controls(), deleteCtrlRecovery, "the recovery window is offered")
	assert.Contains(t, d.View(), "Recovery window", "the adjustable window is drawn")
	assert.Equal(t, defaultDeleteRecoveryWindow, d.effectiveRecoveryWindow(), "staged records the chosen window")

	d.staged = false
	assert.Contains(t, d.controls(), deleteCtrlRecovery, "the recovery window stays visible (the choice is in the popup)")
	assert.Equal(t, 0, d.effectiveRecoveryWindow(), "immediate mode records no custom window")

	d.force = true
	assert.NotContains(t, d.controls(), deleteCtrlRecovery, "forcing hides the recovery window (mutual exclusion)")
	assert.Equal(t, 0, d.effectiveRecoveryWindow(), "a forced delete records no recovery window")
}

// TestDeleteConfirm_DeleteOpensConfirm pins that the Delete button opens the
// Stage/Apply popup (rather than an inline mode row), and Enter there runs the
// delete with the chosen mode — here Apply immediately (→), which writes unstaged.
func TestDeleteConfirm_DeleteOpensConfirm(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	d.name = "prod/key"

	d.focusControl(deleteCtrlDelete)
	_, _ = d.activate()

	require.True(t, d.confirming, "the Delete button opens the Stage/Apply popup")
	assert.False(t, d.busy, "opening the popup does not yet delete")
	assert.Contains(t, d.View(), "Apply immediately", "the popup offers the mode choice")

	_, _ = d.Update(keyRight()) // choose Apply immediately

	m, cmd := d.Update(keyEnter())
	d, ok := m.(*deleteConfirm)
	require.True(t, ok)
	require.NotNil(t, cmd, "enter in the popup emits the delete command")
	assert.True(t, d.busy)

	execCmd(t, cmd)

	got, ok := d.mutator.(*fakeMutator)
	require.True(t, ok)
	assert.True(t, got.deleteCalled)
	assert.False(t, got.staged, "Apply immediately deletes without staging")
}

// TestDeleteConfirm_ConfirmBackReturnsToControls pins that esc in the popup returns
// to the delete controls without deleting.
func TestDeleteConfirm_ConfirmBackReturnsToControls(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	d.focusControl(deleteCtrlDelete)
	_, _ = d.activate()
	require.True(t, d.confirming)

	_, _ = d.Update(keyEsc())
	assert.False(t, d.confirming, "esc dismisses the popup")
	assert.False(t, d.busy, "esc does not delete")

	got, ok := d.mutator.(*fakeMutator)
	require.True(t, ok)
	assert.False(t, got.deleteCalled, "esc attempts no delete")
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
	assert.Equal(t, defaultDeleteRecoveryWindow, got.recoveryWindow)
	assert.Equal(t, "prod/key", got.key.Name)
}

// TestDeleteConfirm_ConfirmGating pins that the Delete button opens the Stage/Apply
// popup only when the service supports staging; without staging the delete is
// immediate and runs directly (no choice to confirm).
func TestDeleteConfirm_ConfirmGating(t *testing.T) {
	t.Parallel()

	staged := newDelete(t, awsSecretCap())
	require.True(t, staged.staged)
	staged.focusControl(deleteCtrlDelete)
	_, _ = staged.activate()
	assert.True(t, staged.confirming, "a staging service opens the popup")
	assert.False(t, staged.busy, "opening the popup does not yet delete")

	noStaging := newDelete(t, capability.ServiceCapability{Service: "secret"})
	require.False(t, noStaging.staged)
	noStaging.focusControl(deleteCtrlDelete)
	_, cmd := noStaging.activate()
	assert.False(t, noStaging.confirming, "without staging there is no choice to confirm")
	assert.True(t, noStaging.busy, "without staging the delete runs directly")
	require.NotNil(t, cmd, "without staging the delete command is dispatched")
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

// TestDeleteConfirm_MouseClickControls pins #663's delete-dialog coverage: a
// click on the force checkbox, the mode radio, the Delete button, and Cancel each
// reduces to the same action navigating to the control and pressing enter/space
// performs, with coordinates from the drawn control regions.
func TestDeleteConfirm_MouseClickControls(t *testing.T) {
	t.Parallel()

	sized := func() *deleteConfirm {
		d := newDelete(t, awsSecretCap())
		_, _ = d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		_ = d.View()

		return d
	}

	// Cancel click cancels.
	d := sized()
	_, cmd := d.Update(clickAt(t, d.hits, deleteControlID(deleteCtrlCancel)))
	require.NotNil(t, cmd)
	_, ok := cmd().(CanceledMsg)
	assert.True(t, ok, "clicking Cancel cancels")

	// Force checkbox click toggles force (like space/enter on it).
	d = sized()
	require.False(t, d.force)
	_, _ = d.Update(clickAt(t, d.hits, deleteControlID(deleteCtrlForce)))
	assert.True(t, d.force, "clicking the force checkbox toggles it")

	// Delete button click opens the Stage/Apply popup (staging service).
	d = sized()
	_, _ = d.Update(clickAt(t, d.hits, deleteControlID(deleteCtrlDelete)))
	assert.True(t, d.confirming, "clicking Delete opens the Stage/Apply popup")
	assert.False(t, d.busy, "clicking Delete does not yet delete")
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

// TestDeleteConfirm_InterceptEsc pins that the delete dialog opts into the shell's
// Esc forwarding (so it owns Esc for the Stage/Apply popup return-vs-cancel).
func TestDeleteConfirm_InterceptEsc(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	assert.True(t, d.InterceptEsc(), "the delete dialog owns Esc")
}

// TestDeleteConfirm_ClampRecovery pins the recovery-window bounds (AWS 7–30): a
// value below 7 clamps to 7, above 30 clamps to 30, and an in-range value is
// unchanged — including the exact edges.
func TestDeleteConfirm_ClampRecovery(t *testing.T) {
	t.Parallel()

	assert.Equal(t, minDeleteRecoveryWindow, clampDeleteRecovery(minDeleteRecoveryWindow-1), "below the floor clamps up to 7")
	assert.Equal(t, minDeleteRecoveryWindow, clampDeleteRecovery(0), "far below the floor clamps to 7")
	assert.Equal(t, minDeleteRecoveryWindow, clampDeleteRecovery(minDeleteRecoveryWindow), "the floor is unchanged")
	assert.Equal(t, 15, clampDeleteRecovery(15), "an in-range value is unchanged")
	assert.Equal(t, maxDeleteRecoveryWindow, clampDeleteRecovery(maxDeleteRecoveryWindow), "the ceiling is unchanged")
	assert.Equal(t, maxDeleteRecoveryWindow, clampDeleteRecovery(maxDeleteRecoveryWindow+1), "above the ceiling clamps down to 30")
}

// TestDeleteConfirm_Adjust pins the recovery-window nudge: ←/→ change the window
// (clamped to the bounds) ONLY while the recovery row is focused; on any other
// control the nudge is inert, and forcing (which hides the recovery row) makes it
// inert too.
func TestDeleteConfirm_Adjust(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	require.Equal(t, defaultDeleteRecoveryWindow, d.recoveryWindow, "starts at the 30-day default")

	// Focus the recovery row: a decrement lowers the window, and it clamps at 7.
	d.focusControl(deleteCtrlRecovery)
	require.Equal(t, deleteCtrlRecovery, d.focused(), "the recovery row is focused")

	d.adjust(-1)
	assert.Equal(t, defaultDeleteRecoveryWindow-1, d.recoveryWindow, "a left nudge lowers the window")

	for range 40 {
		d.adjust(-1)
	}

	assert.Equal(t, minDeleteRecoveryWindow, d.recoveryWindow, "the window clamps at the 7-day floor")

	d.adjust(1)
	assert.Equal(t, minDeleteRecoveryWindow+1, d.recoveryWindow, "a right nudge raises the window")

	// On a non-recovery control the nudge is inert.
	d.focusControl(deleteCtrlDelete)
	before := d.recoveryWindow
	d.adjust(1)
	assert.Equal(t, before, d.recoveryWindow, "the nudge is inert unless the recovery row is focused")
}

// TestDeleteConfirm_LongNameWrapsMinSize pins the safety fix: a long delete
// target name wraps within the dialog width instead of clipping at the screen
// edge (which could hide the suffix that distinguishes sibling paths), while the
// action buttons and the close hint stay on-screen at the minimum size.
func TestDeleteConfirm_LongNameWrapsMinSize(t *testing.T) {
	t.Parallel()

	const name = "/prod/service/database/primary/credentials/password-rotation-key"

	m := NewDeleteConfirm(DeleteInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(), Name: name,
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	view := m.View()

	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"the wrapped name (and hint) never overflow the dialog width")
	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"the whole dialog fits within the terminal height")
	assert.Contains(t, flatten(view), name,
		"the full name is present, wrapped across lines rather than truncated")
	assert.Contains(t, stripANSI(view), "Cancel", "the Cancel button stays on-screen")
	assert.Contains(t, stripANSI(view), "esc: cancel", "the close hint stays on-screen")

	// Before a size is seeded the name is not wrapped (renders as one line): the
	// wrap is driven by the terminal size the shell fans in.
	unsized := NewDeleteConfirm(DeleteInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(), Name: name,
	})
	assert.Contains(t, unsized.View(), name, "unsized: the name renders as a single line")
}

// TestDeleteConfirm_LongErrorStaysBounded pins that a long provider error after
// a failed delete is wrapped and capped so the delete confirm still fits the
// minimum size: the Cancel button and the close hint stay on-screen instead of
// being pushed off the bottom by a tall error (the delete confirm cannot scroll
// because its controls need focus).
func TestDeleteConfirm_LongErrorStaysBounded(t *testing.T) {
	t.Parallel()

	m := NewDeleteConfirm(DeleteInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(),
		Name: "/prod/service/database/primary/credentials/password-rotation-key",
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	d, ok := m.(*deleteConfirm)
	require.True(t, ok)

	d.err = "AccessDeniedException: the caller is not authorized to perform " +
		"secretsmanager:DeleteSecret on this resource because no identity-based policy " +
		"grants the action; contact your administrator to grant the permission or assume " +
		"a role that has it before retrying the delete operation."

	view := m.View()
	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"a long error keeps the whole dialog within the terminal height")
	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"the wrapped error never overflows the dialog width")
	assert.Contains(t, stripANSI(view), "Cancel", "the Cancel button stays on-screen under a long error")
	assert.Contains(t, stripANSI(view), "esc: cancel", "the close hint stays on-screen under a long error")
}
