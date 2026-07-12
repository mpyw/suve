//nolint:testpackage // white-box: drives the apply dialog's Update/fan-out and inspects its state
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

// stubStaging is a controllable data.StagingService for the apply/reset dialog
// tests: Apply returns a preset result (or a conflict result until conflicts are
// ignored) and records the ignoreConflicts flag each call was made with.
type stubStaging struct {
	service string
	label   string

	// result is returned by Apply when ignoreConflicts is honored (or always,
	// when conflict is empty).
	result data.StagingApplyResult
	// conflict, when non-empty, is returned by Apply while ignoreConflicts is
	// false — modelling a conflict rejection that clears once conflicts are
	// ignored.
	conflict data.StagingApplyResult

	applied []bool

	// resetResult is returned by Reset; resets counts how many times Reset ran
	// (so a fan-out test can assert every target was reset).
	resetResult data.StagingResetResult
	resets      int
}

func (s *stubStaging) Service() string { return s.service }
func (s *stubStaging) Label() string   { return s.label }
func (s *stubStaging) Capability() capability.ServiceCapability {
	return capability.ServiceCapability{}
}

func (s *stubStaging) Apply(_ context.Context, ignoreConflicts bool) (data.StagingApplyResult, error) {
	s.applied = append(s.applied, ignoreConflicts)

	if !ignoreConflicts && len(s.conflict.Conflicts) > 0 {
		return s.conflict, nil
	}

	return s.result, nil
}

func (s *stubStaging) Review(context.Context) (data.StagingReview, error) {
	return data.StagingReview{}, nil
}
func (s *stubStaging) Reset(context.Context) (data.StagingResetResult, error) {
	s.resets++

	return s.resetResult, nil
}
func (s *stubStaging) Unstage(context.Context, data.StagedKey) error              { return nil }
func (s *stubStaging) CancelAddTag(context.Context, data.StagedKey, string) error { return nil }
func (s *stubStaging) CancelRemoveTag(context.Context, data.StagedKey, string) error {
	return nil
}

// drive runs the dialog's returned command (if any) and feeds its message back,
// returning the updated dialog.
func drive(t *testing.T, d Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return d
	}

	next, _ := d.Update(cmd())

	return next
}

// pressEnter sends an enter key press.
func pressEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

// pressDown sends a down key press.
func pressDown() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyDown} }

// TestApply_FanOutAggregation pins the global apply-all fan-out: one ApplyUseCase
// per target service, aggregated client-side into a single results view (Azure's
// param and secret have independent scopes, so this must combine per service).
func TestApply_FanOutAggregation(t *testing.T) {
	t.Parallel()

	param := &stubStaging{service: "param", label: "Param", result: data.StagingApplyResult{
		ServiceLabel: "Param",
		Entries:      []data.ApplyEntryResult{{Name: "/a", Status: "updated"}},
	}}
	secret := &stubStaging{service: "secret", label: "Secret", result: data.StagingApplyResult{
		ServiceLabel: "Secret",
		Entries:      []data.ApplyEntryResult{{Name: "s1", Status: "created"}},
	}}

	d := NewApply(ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{param, secret},
		TargetLine: "aws", Title: "Apply staged changes — all", EntryCount: 2, Styles: styles.New(),
	})

	// Focus the Apply button (row 1) and confirm.
	d, _ = d.Update(pressDown())
	d, cmd := d.Update(pressEnter())
	require.True(t, d.Busy(), "the dialog is busy while applying")

	d = drive(t, d, cmd) // run the fan-out command and fold in the results

	assert.Equal(t, []bool{false}, param.applied, "param applied once")
	assert.Equal(t, []bool{false}, secret.applied, "secret applied once")

	view := d.View()
	assert.Contains(t, view, "/a", "the param result is shown")
	assert.Contains(t, view, "s1", "the secret result is shown")
	assert.Contains(t, view, "Param", "results are grouped by service")
	assert.Contains(t, view, "Secret")
}

// TestApply_ConflictThenIgnoreReapply pins the conflict → re-apply path: the
// first apply is rejected with a conflict, and re-applying with "Ignore
// conflicts" enabled overwrites and succeeds.
func TestApply_ConflictThenIgnoreReapply(t *testing.T) {
	t.Parallel()

	svc := &stubStaging{
		service: "param", label: "Param",
		conflict: data.StagingApplyResult{ServiceLabel: "Param", Conflicts: []string{"/app/api/REDIS_URL"}},
		result:   data.StagingApplyResult{ServiceLabel: "Param", Entries: []data.ApplyEntryResult{{Name: "/app/api/REDIS_URL", Status: "updated"}}},
	}

	d := NewApply(ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{svc},
		TargetLine: "aws", Title: "Apply staged changes — Param", EntryCount: 1, Styles: styles.New(),
	})

	// First apply (ignore-conflicts off) → conflict result.
	d, _ = d.Update(pressDown()) // focus Apply
	d, cmd := d.Update(pressEnter())
	d = drive(t, d, cmd)

	assert.Contains(t, d.View(), "conflict", "the first apply reports a conflict")
	assert.Contains(t, d.View(), "Ignore conflicts", "and points to the ignore-conflicts re-apply")

	// Close the results, re-open with ignore-conflicts, and re-apply.
	svc2 := &stubStaging{
		service: "param", label: "Param",
		result: data.StagingApplyResult{ServiceLabel: "Param", Entries: []data.ApplyEntryResult{{Name: "/app/api/REDIS_URL", Status: "updated"}}},
	}
	d2 := NewApply(ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{svc2},
		TargetLine: "aws", Title: "Apply staged changes — Param", EntryCount: 1, Styles: styles.New(),
	})

	// Toggle Ignore conflicts (focus is on the checkbox by default), then Apply.
	d2, _ = d2.Update(pressEnter())   // toggle ignore-conflicts on
	d2, _ = d2.Update(pressDown())    // move to Apply
	d2, cmd = d2.Update(pressEnter()) // confirm
	d2 = drive(t, d2, cmd)

	assert.Equal(t, []bool{true}, svc2.applied, "the re-apply passed ignoreConflicts=true")
	assert.Contains(t, d2.View(), "updated", "the re-apply succeeded")
	assert.NotContains(t, d2.View(), "conflict", "no conflict on the ignore-conflicts re-apply")
}

// TestApply_DismissReloadsOnResults pins that closing the apply dialog with Back
// (Esc) reloads only once it has applied: in the results phase DismissCmd emits
// a MutationDoneMsg (so the shell pops+reloads+voices, matching enter), while in
// the confirm phase it returns nil (a bare cancel, nothing applied yet).
func TestApply_DismissReloadsOnResults(t *testing.T) {
	t.Parallel()

	svc := &stubStaging{
		service: "param", label: "Param",
		result: data.StagingApplyResult{
			ServiceLabel: "Param",
			Entries:      []data.ApplyEntryResult{{Name: "a", Status: "updated"}},
		},
	}

	d := NewApply(ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{svc},
		Title: "Apply staged changes — Param", EntryCount: 1, Styles: styles.New(),
	})

	dr, ok := d.(DismissReloader)
	require.True(t, ok, "the apply dialog is a DismissReloader")
	assert.Nil(t, dr.DismissCmd(), "confirm phase: Back is a bare cancel")

	d, _ = d.Update(pressDown()) // focus Apply
	d, cmd := d.Update(pressEnter())
	require.NotNil(t, cmd)
	d, _ = d.Update(cmd()) // deliver applyResultsMsg → results phase

	dr, ok = d.(DismissReloader)
	require.True(t, ok)

	reload := dr.DismissCmd()
	require.NotNil(t, reload, "results phase: Back reloads")

	done, ok := reload().(MutationDoneMsg)
	require.True(t, ok, "results-phase Back emits MutationDoneMsg (pop+reload+voice)")
	assert.Contains(t, done.Status, "Applied", "the outcome is voiced, matching enter")
}

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
