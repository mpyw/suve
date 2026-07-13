//nolint:testpackage // white-box: pushes a dialog through the real pushDialog so it is wrapped in dialogAdapter
package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/styles"
)

// reloadlessDialog is a test-only dialogs.Model that does NOT implement
// dialogs.DismissReloader, so Esc on it must bare-pop (no reload command).
type reloadlessDialog struct{}

func (reloadlessDialog) Update(tea.Msg) (dialogs.Model, tea.Cmd) { return reloadlessDialog{}, nil }
func (reloadlessDialog) View() string                            { return "reloadless" }
func (reloadlessDialog) Busy() bool                              { return false }

// resultsPhaseApply builds a real apply dialog and drives it through
// confirm → apply → results via the dialog's own public Update, returning the
// dialogs.Model in its results phase. Reaching the results phase this way (not
// by poking unexported fields) mirrors how the shell actually acquires it.
func resultsPhaseApply(t *testing.T) dialogs.Model {
	t.Helper()

	svc := &goldenStaging{
		service: "param", label: "Param",
		applyResult: data.StagingApplyResult{
			ServiceLabel: "Param",
			Entries:      []data.ApplyEntryResult{{Name: "/app/web/CDN_URL", Status: "updated"}},
		},
	}

	d := dialogs.NewApply(dialogs.ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{svc},
		TargetLine: string(provider.ProviderAWS), Title: "Apply staged changes — Param", EntryCount: 1, Styles: styles.New(),
	})

	d, _ = d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	d, _ = d.Update(tea.KeyPressMsg{Code: tea.KeyDown})     // focus the Apply button
	d, cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // confirm → busy, returns the fan-out cmd
	require.NotNil(t, cmd, "confirming apply returns the fan-out command")
	require.True(t, d.Busy(), "the dialog is busy while applying")

	d, _ = d.Update(cmd()) // fold the applyResultsMsg back in → results phase
	require.False(t, d.Busy(), "the dialog left the busy phase once results arrived")

	return d
}

// TestUpdate_EscOnApplyResultsReloadsThroughAdapter pins #744: the app stores
// every dialog wrapped in dialogAdapter, so the shell's Back handler asserts
// dialogs.DismissReloader against the ADAPTER, not the raw *applyDialog. The
// adapter must forward DismissCmd so Esc on the apply-results view emits
// MutationDoneMsg (the post-apply reload), instead of bare-popping and leaving
// the staging sections + tab badge stale. The dialog-package tests missed this
// because they assert DismissReloader on the raw *applyDialog, never through the
// adapter the app actually pushes.
func TestUpdate_EscOnApplyResultsReloadsThroughAdapter(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})

	// pushDialog wraps the dialog in dialogAdapter — the exact production wiring.
	m.pushDialog(resultsPhaseApply(t), nil)
	require.Len(t, m.dialogs, 1, "the results dialog is on the stack")

	next, cmd := m.Update(specialKey(tea.KeyEscape))
	m, ok := next.(*App)
	require.True(t, ok, "Update returns *App")

	require.NotNil(t, cmd, "Esc on the results view must emit the reload command, not bare-pop")
	require.Len(t, m.dialogs, 1, "Esc does not bare-pop; the pop happens when MutationDoneMsg is handled")

	done, ok := cmd().(dialogs.MutationDoneMsg)
	require.True(t, ok, "the reload command emits MutationDoneMsg (the post-apply reload), not a bare pop")
	assert.True(t, done.Staged, "the apply-results dismiss voices the applied outcome")

	// Feeding the emitted MutationDoneMsg back pops the dialog (the reload path).
	m = updateApp(t, m, done)
	assert.Empty(t, m.dialogs, "handling MutationDoneMsg closes the results dialog")
}

// TestUpdate_EscOnNonReloaderBarePops pins the other side of #744: a dialog that
// is NOT a dialogs.DismissReloader still bare-pops on Esc through the adapter —
// the forwarded DismissCmd returns nil, so the shell falls back to the plain pop
// with no reload command.
func TestUpdate_EscOnNonReloaderBarePops(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	m.pushDialog(reloadlessDialog{}, nil)
	require.Len(t, m.dialogs, 1, "the reloadless dialog is on the stack")

	next, cmd := m.Update(specialKey(tea.KeyEscape))
	m, ok := next.(*App)
	require.True(t, ok, "Update returns *App")

	assert.Empty(t, m.dialogs, "Esc bare-pops a non-DismissReloader dialog")
	assert.Nil(t, cmd, "a bare pop emits no reload command")
}
