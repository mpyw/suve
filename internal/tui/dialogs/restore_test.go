//nolint:testpackage // white-box: drives the restore form's routing and result branches directly
package dialogs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

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
