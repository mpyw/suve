//nolint:testpackage // white-box: drives the apply dialog's Update/fan-out and inspects its state
package dialogs

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// applyAllStub builds one target of a multi-target apply: a stub whose use case
// applies an update of name over a real strategy and the shared staging store.
// The remote was last modified at modified, so a modification after the staged
// base time is a conflict. puts records the names written to the remote.
func applyAllStub(
	t *testing.T, mem *testutil.MockStore, svc staging.Service, label, name string, conflict bool, puts *[]string,
) *stubStaging {
	t.Helper()

	base := time.Now().Add(-time.Hour)
	modified := base.Add(-time.Hour)

	if conflict {
		modified = time.Now()
	}

	remote := &providermock.Store{
		GetFunc: func(_ context.Context, got string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: got, Value: "remote", Version: domain.Version{ID: "1"}, Modified: new(modified)}, nil
		},
		PutFunc: func(_ context.Context, got, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
			*puts = append(*puts, got)

			return domain.Version{ID: "2"}, nil
		},
	}

	var strategy staging.FullStrategy = staging.NewAWSParamStrategy(remote)
	if svc == staging.ServiceSecret {
		strategy = staging.NewAWSSecretStrategy(remote)
	}

	require.NoError(t, mem.StageEntry(t.Context(), svc, staging.EntryKey{Name: name}, staging.Entry{
		Operation: staging.OperationUpdate, Value: lo.ToPtr("staged"), StagedAt: time.Now(), BaseModifiedAt: &base,
	}))

	return &stubStaging{
		service: string(svc), label: label,
		useCase: &stagingusecase.ApplyUseCase{Strategy: strategy, Store: mem},
	}
}

// runApplyAll confirms a multi-target apply dialog and returns it in its results
// phase.
func runApplyAll(t *testing.T, targets ...data.StagingService) Model {
	t.Helper()

	d := NewApply(ApplyInput{
		Ctx: t.Context(), Targets: targets,
		TargetLine: "aws", Title: "Apply staged changes — all", EntryCount: len(targets), Styles: styles.New(),
	})

	// Focus the Apply button (row 1) and confirm.
	d, _ = d.Update(pressDown())
	d, cmd := d.Update(pressEnter())
	require.True(t, d.Busy(), "the dialog is busy while applying")

	return drive(t, d, cmd) // run the apply command and fold in the results
}

// TestApply_FanOutAggregation pins the apply-all: every target is applied as one
// all-service apply, and the per-service results render as one results view
// grouped by service.
func TestApply_FanOutAggregation(t *testing.T) {
	t.Parallel()

	var puts []string

	mem := testutil.NewMockStore()
	param := applyAllStub(t, mem, staging.ServiceParam, "Param", "/a", false, &puts)
	secret := applyAllStub(t, mem, staging.ServiceSecret, "Secret", "s1", false, &puts)

	d := runApplyAll(t, param, secret)

	assert.Empty(t, param.applied, "a multi-target apply does not apply target by target")
	assert.Empty(t, secret.applied)
	assert.Equal(t, []string{"/a", "s1"}, puts, "both services are written")

	view := d.View()
	assert.Contains(t, view, "/a", "the param result is shown")
	assert.Contains(t, view, "s1", "the secret result is shown")
	assert.Contains(t, view, "Param", "results are grouped by service")
	assert.Contains(t, view, "Secret")
}

// TestApply_AllRejectsWhenOneServiceConflicts pins #982: with Ignore conflicts
// off, a conflict in one service rejects the whole apply-all, so the
// conflict-free service is neither written nor unstaged.
func TestApply_AllRejectsWhenOneServiceConflicts(t *testing.T) {
	t.Parallel()

	var puts []string

	mem := testutil.NewMockStore()
	param := applyAllStub(t, mem, staging.ServiceParam, "Param", "/a", false, &puts)
	secret := applyAllStub(t, mem, staging.ServiceSecret, "Secret", "s1", true, &puts)

	d := runApplyAll(t, param, secret)

	assert.Empty(t, puts, "no service is written")

	_, err := mem.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/a"})
	require.NoError(t, err, "the conflict-free param stays staged")

	view := d.View()
	assert.Contains(t, view, "apply rejected: no service was applied")
	assert.Contains(t, view, "Secret", "the conflicting service is named")
	assert.Contains(t, view, "conflict: s1")
	assert.NotContains(t, view, "✓", "nothing is reported as applied")

	ad, ok := d.(*applyDialog)
	require.True(t, ok)
	assert.Equal(t, "Apply rejected: 1 conflict(s). Re-apply with Ignore conflicts to overwrite.", ad.summary())
}

// manyApplyEntries builds n distinct applied-entry results, named entry-000…entry-NNN
// so a test can locate the first vs last in the rendered viewport.
func manyApplyEntries(n int) []data.ApplyEntryResult {
	entries := make([]data.ApplyEntryResult, n)
	for i := range entries {
		entries[i] = data.ApplyEntryResult{Name: fmt.Sprintf("entry-%03d", i), Status: "updated"}
	}

	return entries
}

// appliedResults builds an apply dialog, sizes it to a fixed 100×24 terminal, and
// drives it through the confirm → apply → results transition with the given
// single-service result, returning the concrete dialog in its results phase. The
// 24-row height is short enough that a many-entry body must scroll.
func appliedResults(t *testing.T, result data.StagingApplyResult) *applyDialog {
	t.Helper()

	const (
		width  = 100
		height = 24
	)

	svc := &stubStaging{service: "param", label: "Param", result: result}

	m := NewApply(ApplyInput{
		Ctx: context.Background(), Targets: []data.StagingService{svc},
		TargetLine: "aws", Title: "Apply staged changes — Param", EntryCount: len(result.Entries), Styles: styles.New(),
	})

	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(pressDown()) // focus Apply
	m, cmd := m.Update(pressEnter())
	m = drive(t, m, cmd) // run the fan-out command and fold in the results

	d, ok := m.(*applyDialog)
	require.True(t, ok, "the apply dialog is an *applyDialog")
	require.Equal(t, applyPhaseResults, d.phase, "the dialog reached the results phase")

	return d
}

// TestApply_ResultsScrollable pins the #687 fix: a result set taller than the box
// is capped into a scrollable viewport, the close hint stays pinned (never clipped
// off-screen), and paging down reaches the tail that was hidden at the top.
func TestApply_ResultsScrollable(t *testing.T) {
	t.Parallel()

	const entries = 50

	// A short terminal: the 50-line body cannot fit, so it must scroll.
	d := appliedResults(t, data.StagingApplyResult{
		ServiceLabel: "Param", Entries: manyApplyEntries(entries),
	})

	assert.True(t, d.scrollable, "a body taller than the box scrolls")
	assert.Equal(t, entries, d.vp.TotalLineCount(), "every result line is in the viewport")
	assert.Less(t, d.vp.Height(), entries, "the viewport is capped below the full body height")
	assert.True(t, d.vp.AtTop(), "the results open scrolled to the top")

	top := d.View()
	assert.Contains(t, top, "enter/esc: close", "the close hint is pinned even with a long body")
	assert.Contains(t, top, "scroll", "the hint advertises the scroll keys when the body overflows")
	assert.Contains(t, top, "entry-000", "the first result is visible at the top")
	assert.NotContains(t, top, "entry-049", "the last result is below the fold at the top")

	// Page down until the tail is reached (a bounded loop: each page advances by
	// the viewport height, so the body is exhausted well within `entries` presses).
	// Update mutates the pointer receiver in place, so d reflects each scroll.
	for range entries {
		if d.vp.AtBottom() {
			break
		}

		_, _ = d.Update(pressPgDown())
	}

	assert.True(t, d.vp.AtBottom(), "paging down reaches the bottom of the results")

	bottom := d.View()
	assert.Contains(t, bottom, "entry-049", "the previously-hidden last result is reachable by scrolling")
	assert.Contains(t, bottom, "enter/esc: close", "the close hint stays pinned after scrolling")
}

// TestApply_ResultsMouseWheelScrolls pins that the mouse wheel scrolls the results
// body (the #687 report noted the wheel was dropped): a wheel-down event advances
// the viewport off the top.
func TestApply_ResultsMouseWheelScrolls(t *testing.T) {
	t.Parallel()

	d := appliedResults(t, data.StagingApplyResult{
		ServiceLabel: "Param", Entries: manyApplyEntries(50),
	})
	require.True(t, d.vp.AtTop(), "the results open at the top")

	_, _ = d.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	assert.False(t, d.vp.AtTop(), "a wheel-down event scrolls the results body")
}

// TestApply_ResultsShortNoScroll pins that a body that fits renders inline (no
// scroll): the viewport is sized to the exact body height, nothing is clipped,
// and the hint does not advertise scroll keys that would do nothing.
func TestApply_ResultsShortNoScroll(t *testing.T) {
	t.Parallel()

	d := appliedResults(t, data.StagingApplyResult{
		ServiceLabel: "Param",
		Entries: []data.ApplyEntryResult{
			{Name: "entry-000", Status: "updated"},
			{Name: "entry-001", Status: "created"},
		},
	})

	assert.False(t, d.scrollable, "a body that fits does not scroll")
	assert.Equal(t, 2, d.vp.TotalLineCount(), "both result lines are present")

	view := d.View()
	assert.Contains(t, view, "entry-000", "the first result renders")
	assert.Contains(t, view, "entry-001", "the second result renders")
	assert.Contains(t, view, "enter/esc: close", "the close hint renders")
	assert.NotContains(t, view, "scroll", "no scroll keys are advertised when the body fits")
}

// TestApply_ScrollThenDismissReloads pins that the #669 Esc-reload behavior
// survives scrolling: after paging through a long results list, dismissing with
// Back (DismissCmd) still emits the pop+reload+voice MutationDoneMsg.
func TestApply_ScrollThenDismissReloads(t *testing.T) {
	t.Parallel()

	d := appliedResults(t, data.StagingApplyResult{
		ServiceLabel: "Param", Entries: manyApplyEntries(50),
	})

	// Scroll down; Esc (routed by the shell to DismissCmd) must still reload.
	m, _ := d.Update(pressPgDown())
	dr, ok := m.(DismissReloader)
	require.True(t, ok, "the apply dialog is a DismissReloader after scrolling")

	reload := dr.DismissCmd()
	require.NotNil(t, reload, "results phase: Back still reloads after scrolling")

	done, ok := reload().(MutationDoneMsg)
	require.True(t, ok, "Back after scrolling emits MutationDoneMsg (pop+reload+voice)")
	assert.Contains(t, done.Status, "Applied", "the outcome is still voiced")
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

// TestApply_MouseClickConfirmControls pins #663's confirm-dialog coverage: a
// click on the Ignore checkbox, Apply, and Cancel reduces to the same action the
// key path performs (toggle, confirm, cancel), with coordinates from the drawn
// control regions.
func TestApply_MouseClickConfirmControls(t *testing.T) {
	t.Parallel()

	newDialog := func() *applyDialog {
		svc := &stubStaging{service: "param", label: "Param", result: data.StagingApplyResult{ServiceLabel: "Param"}}
		m := NewApply(ApplyInput{
			Ctx: context.Background(), Targets: []data.StagingService{svc},
			TargetLine: "aws", Title: "Apply staged changes — Param", EntryCount: 1, Styles: styles.New(),
		})
		m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		_ = m.View()
		d, ok := m.(*applyDialog)
		require.True(t, ok)

		return d
	}

	// Ignore checkbox click toggles ignoreConflicts (like enter on it).
	d := newDialog()
	require.False(t, d.ignoreConflicts)
	_, _ = d.Update(clickAt(t, d.hits, applyRegionIgnore))
	assert.True(t, d.ignoreConflicts, "clicking the Ignore checkbox toggles it")

	// Apply button click confirms: busy + fan-out command (like focus Apply + enter).
	d = newDialog()
	_, cmd := d.Update(clickAt(t, d.hits, applyRegionApply))
	assert.True(t, d.Busy(), "clicking Apply starts the fan-out")
	require.NotNil(t, cmd, "clicking Apply dispatches the apply command")

	// Cancel button click cancels.
	d = newDialog()
	_, cmd = d.Update(clickAt(t, d.hits, applyRegionCancel))
	require.NotNil(t, cmd)
	_, ok := cmd().(CanceledMsg)
	assert.True(t, ok, "clicking Cancel cancels")
}

// TestApply_MouseClickResultsCloses pins that clicking the results close hint
// closes with the same reload+voice enter performs.
func TestApply_MouseClickResultsCloses(t *testing.T) {
	t.Parallel()

	d := appliedResults(t, data.StagingApplyResult{
		ServiceLabel: "Param",
		Entries:      []data.ApplyEntryResult{{Name: "a", Status: "updated"}},
	})
	_ = d.View() // build the results-phase close region

	_, cmd := d.Update(clickAt(t, d.hits, regionClose))
	require.NotNil(t, cmd, "clicking the close hint dispatches")
	done, ok := cmd().(MutationDoneMsg)
	require.True(t, ok, "clicking close emits MutationDoneMsg (like enter)")
	assert.Contains(t, done.Status, "Applied", "the outcome is voiced")
}

// TestApply_ResultsKeepEachNamespace pins one results line per namespace: the
// same name applied under the null and "prod" namespaces shows twice, the
// second with its [prod] badge, for entries, tags and unstage warnings.
func TestApply_ResultsKeepEachNamespace(t *testing.T) {
	t.Parallel()

	d := appliedResults(t, data.StagingApplyResult{
		ServiceLabel: "App Configuration",
		Entries: []data.ApplyEntryResult{
			{Name: "/app/config", Status: "updated"},
			{Name: "/app/config", Namespace: "prod", Status: "updated", UnstageError: "locked"},
		},
		Tags: []data.ApplyTagResult{
			{Name: "/app/config", Namespace: "prod"},
		},
	})

	// Drop the SGR styling so the assertions read the plain lines.
	body := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(d.resultsBody(), "")
	assert.Contains(t, body, "✓ updated  /app/config\n")
	assert.Contains(t, body, "✓ updated  /app/config [prod]")
	assert.Contains(t, body, "✓ tags  /app/config [prod]")
	assert.Contains(t, body, "⚠ /app/config [prod] applied but could not be unstaged")
}
