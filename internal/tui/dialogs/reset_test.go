//nolint:testpackage // white-box: drives the reset dialog's fan-out, voicing, and buttons directly
package dialogs

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// TestReset_FanOutAggregation pins that reset-all fans out one reset per service
// and voices the combined unstaged count.
func TestReset_FanOutAggregation(t *testing.T) {
	t.Parallel()

	param := &stubStaging{
		service: "param", label: "Param",
		resetResult: data.StagingResetResult{Type: data.StagingResetUnstagedAll, Count: 2},
	}
	secret := &stubStaging{
		service: "secret", label: "Secret",
		resetResult: data.StagingResetResult{Type: data.StagingResetUnstagedAll, Count: 3},
	}

	d := NewReset(ResetInput{
		Ctx: context.Background(), Targets: []data.StagingService{param, secret},
		Title: "Reset staged changes — all", Styles: styles.New(),
	})

	d, _ = d.Update(pressDown()) // focus defaults to Cancel; move to Reset
	d, cmd := d.Update(pressEnter())
	require.True(t, d.Busy())
	require.NotNil(t, cmd)

	next, doneCmd := d.Update(cmd())

	assert.Equal(t, 1, param.resets, "param was reset exactly once")
	assert.Equal(t, 1, secret.resets, "secret was reset exactly once")

	require.NotNil(t, doneCmd)
	done, ok := doneCmd().(MutationDoneMsg)
	require.True(t, ok, "reset emits a done message")
	assert.Contains(t, done.Status, "5", "the aggregate voices the summed unstaged count (2+3)")
	assert.False(t, next.Busy(), "the dialog clears busy once the reset finishes")
}

// TestReset_MouseClickButtons pins that clicking the Reset/Cancel buttons reduces
// to the same action the key path performs.
func TestReset_MouseClickButtons(t *testing.T) {
	t.Parallel()

	newDialog := func() *resetDialog {
		svc := &stubStaging{
			service: "param", label: "Param",
			resetResult: data.StagingResetResult{Type: data.StagingResetUnstagedAll, Count: 1},
		}
		m := NewReset(ResetInput{
			Ctx: context.Background(), Targets: []data.StagingService{svc},
			Title: "Reset staged changes — Param", Styles: styles.New(),
		})
		m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		_ = m.View()
		d, ok := m.(*resetDialog)
		require.True(t, ok)

		return d
	}

	// Cancel click cancels.
	d := newDialog()
	_, cmd := d.Update(clickAt(t, d.hits, resetRegionCancel))
	require.NotNil(t, cmd)
	_, ok := cmd().(CanceledMsg)
	assert.True(t, ok, "clicking Cancel cancels")

	// Reset click confirms: busy + reset command.
	d = newDialog()
	_, cmd = d.Update(clickAt(t, d.hits, resetRegionReset))
	assert.True(t, d.Busy(), "clicking Reset starts the reset")
	require.NotNil(t, cmd, "clicking Reset dispatches the reset command")
}

// TestReset_DefaultFocusCancels pins that the reset confirm opens focused on
// Cancel, so an accidental enter (e.g. an "R enter" double-tap) cancels instead
// of wiping staged changes — parity with the delete/apply confirms.
func TestReset_DefaultFocusCancels(t *testing.T) {
	t.Parallel()

	param := &stubStaging{service: "param", label: "Param"}

	d := NewReset(ResetInput{
		Ctx: context.Background(), Targets: []data.StagingService{param},
		Title: "Reset staged changes — Param", Styles: styles.New(),
	})

	_, cmd := d.Update(pressEnter()) // enter on the default focus

	require.NotNil(t, cmd)
	_, ok := cmd().(CanceledMsg)
	assert.True(t, ok, "enter on the default focus cancels")
	assert.Equal(t, 0, param.resets, "no reset ran")
}

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
