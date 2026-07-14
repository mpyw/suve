//nolint:testpackage // white-box: drives the dialogs' result/clamp/intercept branches directly
package dialogs

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// TestResetTypeStatus pins the per-ResetType voicing of a single-target reset:
// every StagingResetType maps onto its own phrase, and an unknown type falls back
// to the bare "Reset.".
func TestResetTypeStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		res  data.StagingResetResult
		want string
	}{
		{"unstaged-all", data.StagingResetResult{Type: data.StagingResetUnstagedAll, Count: 3}, "Unstaged 3 staged change(s)."},
		{"nothing-staged", data.StagingResetResult{Type: data.StagingResetNothingStaged}, "Nothing staged."},
		{"unstaged", data.StagingResetResult{Type: data.StagingResetUnstaged}, "Unstaged the staged change."},
		{"unstaged-tag", data.StagingResetResult{Type: data.StagingResetUnstagedTag}, "Unstaged the staged tag change."},
		{"restored", data.StagingResetResult{Type: data.StagingResetRestored}, "Restored the staged value."},
		{"skipped", data.StagingResetResult{Type: data.StagingResetSkipped}, "Skipped — value matches the current value."},
		{"not-staged", data.StagingResetResult{Type: data.StagingResetNotStaged}, "Not staged."},
		{"unknown", data.StagingResetResult{Type: data.StagingResetType(999)}, "Reset."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, resetTypeStatus(tc.res))
		})
	}
}

// TestReset_ResultVoicing pins that a single-target reset result is folded into a
// MutationDoneMsg voicing its exact type, a hard failure closes and voices the
// failure (still reloading so partial resets refresh), and either way the dialog
// clears busy.
func TestReset_ResultVoicing(t *testing.T) {
	t.Parallel()

	svc := &stubStaging{service: "param", label: "Param"}

	success := NewReset(ResetInput{
		Ctx: t.Context(), Targets: []data.StagingService{svc},
		Title: "Reset staged changes — Param", Styles: styles.New(),
	})
	success, _ = success.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	next, cmd := success.Update(resetResultsMsg{
		results: []data.StagingResetResult{{Type: data.StagingResetUnstaged}},
	})
	require.NotNil(t, cmd, "a reset result emits a done command")
	assert.False(t, next.Busy(), "a reset result clears busy")

	done, ok := cmd().(MutationDoneMsg)
	require.True(t, ok, "the result is voiced as a MutationDoneMsg")
	assert.Equal(t, "Unstaged the staged change.", done.Status)

	failed := NewReset(ResetInput{
		Ctx: t.Context(), Targets: []data.StagingService{svc},
		Title: "Reset staged changes — Param", Styles: styles.New(),
	})
	next, cmd = failed.Update(resetResultsMsg{err: errors.New("boom")})
	require.NotNil(t, cmd, "a reset failure still emits a done command (reloads partial successes)")
	assert.False(t, next.Busy(), "a reset failure clears busy")

	done, ok = cmd().(MutationDoneMsg)
	require.True(t, ok)
	assert.Contains(t, done.Status, "Reset failed: boom", "the failure is voiced on the status line")
}

// TestRestore_OnResult pins the restore dialog's result split: a success emits
// the "Restored." MutationDoneMsg (clearing busy), while an error clears busy,
// records the error, and rebuilds the form (returning a command) rather than
// closing.
func TestRestore_OnResult(t *testing.T) {
	t.Parallel()

	newRestore := func() *restoreForm {
		m, _ := NewRestore(RestoreInput{
			Ctx: t.Context(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
			Service: "secret", Styles: styles.New(), Name: "prod/x",
		})

		d, ok := m.(*restoreForm)
		require.True(t, ok)

		d.busy = true

		return d
	}

	ok := newRestore()
	m, cmd := ok.onResult(mutationResultMsg{outcome: data.WriteOutcome{}})
	assert.False(t, m.Busy(), "a successful restore clears busy")
	require.NotNil(t, cmd, "a successful restore emits a done command")
	done, isDone := cmd().(MutationDoneMsg)
	require.True(t, isDone, "success emits MutationDoneMsg")
	assert.Equal(t, "Restored.", done.Status)

	fail := newRestore()
	m, cmd = fail.onResult(mutationResultMsg{err: errors.New("access denied")})
	d, isForm := m.(*restoreForm)
	require.True(t, isForm)
	assert.False(t, d.Busy(), "an error clears busy")
	assert.Equal(t, "access denied", d.err, "the error is recorded for the footer")
	require.NotNil(t, cmd, "an error rebuilds the form (does not close)")
	assert.Contains(t, d.View(), "access denied", "the error is surfaced in the footer")
}

// TestRestore_BusySwallowsInput pins the restore double-submit guard: while busy
// the dialog swallows key input (no form advance) and reports Busy().
func TestRestore_BusySwallowsInput(t *testing.T) {
	t.Parallel()

	m, _ := NewRestore(RestoreInput{
		Ctx: t.Context(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(), Name: "prod/x",
	})

	d, ok := m.(*restoreForm)
	require.True(t, ok)

	d.busy = true

	assert.True(t, d.Busy())

	_, cmd := d.Update(keyMsg('a'))
	assert.Nil(t, cmd, "input is swallowed while busy")
	assert.True(t, d.Busy(), "the dialog stays busy")
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

	assert.Equal(t, minRecoveryWindow, clampRecovery(minRecoveryWindow-1), "below the floor clamps up to 7")
	assert.Equal(t, minRecoveryWindow, clampRecovery(0), "far below the floor clamps to 7")
	assert.Equal(t, minRecoveryWindow, clampRecovery(minRecoveryWindow), "the floor is unchanged")
	assert.Equal(t, 15, clampRecovery(15), "an in-range value is unchanged")
	assert.Equal(t, maxRecoveryWindow, clampRecovery(maxRecoveryWindow), "the ceiling is unchanged")
	assert.Equal(t, maxRecoveryWindow, clampRecovery(maxRecoveryWindow+1), "above the ceiling clamps down to 30")
}

// TestDeleteConfirm_Adjust pins the recovery-window nudge: ←/→ change the window
// (clamped to the bounds) ONLY while the recovery row is focused; on any other
// control the nudge is inert, and forcing (which hides the recovery row) makes it
// inert too.
func TestDeleteConfirm_Adjust(t *testing.T) {
	t.Parallel()

	d := newDelete(t, awsSecretCap())
	require.Equal(t, defaultRecoveryWindow, d.recoveryWindow, "starts at the 30-day default")

	// Focus the recovery row: a decrement lowers the window, and it clamps at 7.
	d.focusControl(ctrlRecovery)
	require.Equal(t, ctrlRecovery, d.focused(), "the recovery row is focused")

	d.adjust(-1)
	assert.Equal(t, defaultRecoveryWindow-1, d.recoveryWindow, "a left nudge lowers the window")

	for range 40 {
		d.adjust(-1)
	}

	assert.Equal(t, minRecoveryWindow, d.recoveryWindow, "the window clamps at the 7-day floor")

	d.adjust(1)
	assert.Equal(t, minRecoveryWindow+1, d.recoveryWindow, "a right nudge raises the window")

	// On a non-recovery control the nudge is inert.
	d.focusControl(ctrlDelete)
	before := d.recoveryWindow
	d.adjust(1)
	assert.Equal(t, before, d.recoveryWindow, "the nudge is inert unless the recovery row is focused")
}

// TestErrorDialog_Update pins the error dialog's message handling: enter/esc
// dismisses (CanceledMsg); a scroll key routes into the viewport when the body
// overflows; and a resize (re)builds the scrollable body.
func TestErrorDialog_Update(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a very long provider error line that must wrap and then scroll inside the box\n", 200)

	m := NewError(styles.New(), "Cannot proceed", long)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})

	d, ok := m.(*errorDialog)
	require.True(t, ok)
	assert.True(t, d.scrollable, "a body taller than the box scrolls")

	// A scroll key (pgdn) advances the viewport off the top (not a dismissal).
	require.True(t, d.vp.AtTop(), "the message opens at the top")
	_, cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.Nil(t, cmd, "a scroll key does not dismiss")
	assert.False(t, d.vp.AtTop(), "a scroll key advances the viewport")

	// Enter dismisses with a CanceledMsg.
	_, cmd = d.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter dispatches")
	_, isCancel := cmd().(CanceledMsg)
	assert.True(t, isCancel, "enter dismisses the error dialog")

	// The mouse wheel scrolls the body too.
	_, _ = d.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
}

// TestErrorDialog_TitleDefaults pins that an empty title falls back to "Error"
// and Busy() is always false (the error dialog never mutates).
func TestErrorDialog_TitleDefaults(t *testing.T) {
	t.Parallel()

	m := NewError(styles.New(), "", "something went wrong")
	d, ok := m.(*errorDialog)
	require.True(t, ok)

	assert.Equal(t, "Error", d.title, "an empty title falls back to \"Error\"")
	assert.False(t, d.Busy(), "the error dialog is never busy")
	assert.Contains(t, d.View(), "something went wrong", "a size-less render shows the whole message inline")
}
