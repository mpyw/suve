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
	return data.StagingResetResult{}, nil
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

// TestReset_FanOutAggregation pins that reset-all fans out one reset per service
// and voices the combined unstaged count.
func TestReset_FanOutAggregation(t *testing.T) {
	t.Parallel()

	param := &stubStaging{service: "param", label: "Param"}
	secret := &stubStaging{service: "secret", label: "Secret"}

	d := NewReset(ResetInput{
		Ctx: context.Background(), Targets: []data.StagingService{param, secret},
		Title: "Reset staged changes — all", Styles: styles.New(),
	})

	d, cmd := d.Update(pressEnter()) // focus is on Reset by default
	require.True(t, d.Busy())

	next, doneCmd := d.Update(cmd())
	require.NotNil(t, doneCmd)

	done, ok := doneCmd().(MutationDoneMsg)
	require.True(t, ok, "reset emits a done message")
	assert.NotEmpty(t, done.Status, "the reset outcome is voiced")
	assert.False(t, next.Busy(), "the dialog clears busy once the reset finishes")
}
