package staging_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	usecasestaging "github.com/mpyw/suve/internal/usecase/staging"
)

func TestApplyUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	require.NoError(t, err)
	assert.Equal(t, "Parameter Store", output.ServiceName)
	assert.Equal(t, "parameter", output.ItemName)
	assert.Empty(t, output.EntryResults)
	assert.Equal(t, 0, output.EntrySucceeded)
	assert.Equal(t, 0, output.EntryFailed)
}

func TestApplyUseCase_Execute_SingleCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("new-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, output.EntrySucceeded)
	assert.Equal(t, 0, output.EntryFailed)
	require.Len(t, output.EntryResults, 1)
	assert.Equal(t, usecasestaging.ApplyResultCreated, output.EntryResults[0].Status)

	// Verify unstaged after apply
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_MultipleOperations(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/create"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("create"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/update"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("update"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/delete"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, output.EntrySucceeded)
	assert.Equal(t, 0, output.EntryFailed)
}

func TestApplyUseCase_Execute_ResultsSorted(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Stage in non-sorted name order; results must come back sorted regardless.
	for _, name := range []string{"/app/charlie", "/app/alpha", "/app/bravo"} {
		require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: name}, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("v"),
			StagedAt:  time.Now(),
		}))
	}

	for _, name := range []string{"/tag/zulu", "/tag/mike", "/tag/delta"} {
		require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: name}, staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}))
	}

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)

	entryNames := make([]string, len(output.EntryResults))
	for i, r := range output.EntryResults {
		entryNames[i] = r.Name
	}

	assert.Equal(t, []string{"/app/alpha", "/app/bravo", "/app/charlie"}, entryNames)

	tagNames := make([]string, len(output.TagResults))
	for i, r := range output.TagResults {
		tagNames[i] = r.Name
	}

	assert.Equal(t, []string{"/tag/delta", "/tag/mike", "/tag/zulu"}, tagNames)
}

func TestApplyUseCase_Execute_FilterByName(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/one"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("one"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/two"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("two"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		Name:            "/app/one",
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, output.EntrySucceeded)
	require.Len(t, output.EntryResults, 1)
	assert.Equal(t, "/app/one", output.EntryResults[0].Name)

	// /app/two should still be staged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/two", Namespace: ""})
	require.NoError(t, err)
}

func TestApplyUseCase_Execute_FilterByName_NotStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		Name: "/app/not-staged",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not staged")
}

func TestApplyUseCase_Execute_PartialFailure(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/success"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("success"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/fail"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("fail"),
		StagedAt:  time.Now(),
	}))

	strategy := newMockApplyStrategy()
	strategy.applyErrors["/app/fail"] = errors.New("aws error")

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied 1 entries")
	assert.Equal(t, 1, output.EntrySucceeded)
	assert.Equal(t, 1, output.EntryFailed)

	// Failed entry should still be staged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/fail", Namespace: ""})
	require.NoError(t, err)

	// Successful entry should be unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/success", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_ConflictDetection(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	awsTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC) // Modified after staging

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/conflict"}, staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          new("staged"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	strategy := newMockApplyStrategy()
	strategy.lastModified["/app/conflict"] = awsTime

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict")
	assert.Len(t, output.Conflicts, 1)
	assert.Equal(t, staging.EntryKey{Name: "/app/conflict"}, output.Conflicts[0])
}

func TestApplyUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListEntriesErr = errors.New("list error")

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

func TestApplyUseCase_Execute_DeleteSuccess(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/to-delete"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, output.EntrySucceeded)
	require.Len(t, output.EntryResults, 1)
	assert.Equal(t, usecasestaging.ApplyResultDeleted, output.EntryResults[0].Status)
}

// =============================================================================
// Tag Apply Tests
// =============================================================================

type mockApplyTagStrategy struct {
	*mockApplyStrategy

	applyTagsErrors map[string]error
}

func (m *mockApplyTagStrategy) ApplyTags(_ context.Context, name string, _ staging.TagEntry) error {
	if err, ok := m.applyTagsErrors[name]; ok {
		return err
	}

	return nil
}

func newMockApplyTagStrategy() *mockApplyTagStrategy {
	return &mockApplyTagStrategy{
		mockApplyStrategy: newMockApplyStrategy(),
		applyTagsErrors:   make(map[string]error),
	}
}

func TestApplyUseCase_Execute_TagsOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyTagStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	require.NoError(t, err)
	assert.Equal(t, 0, output.EntrySucceeded)
	assert.Equal(t, 0, output.EntryFailed)
	assert.Equal(t, 1, output.TagSucceeded)
	assert.Equal(t, 0, output.TagFailed)
	require.Len(t, output.TagResults, 1)
	assert.Equal(t, "/app/config", output.TagResults[0].Name)
	assert.Equal(t, "prod", output.TagResults[0].AddTags["env"])

	// Verify unstaged after apply
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_TagsWithRemove(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	removeKeys := map[string]struct{}{"deprecated": {}, "old": {}}
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		Remove:   removeKeys,
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyTagStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	require.NoError(t, err)
	assert.Equal(t, 1, output.TagSucceeded)
	require.Len(t, output.TagResults, 1)
	assert.True(t, output.TagResults[0].RemoveTag.Contains("deprecated"))
	assert.True(t, output.TagResults[0].RemoveTag.Contains("old"))
}

func TestApplyUseCase_Execute_EntriesAndTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("new-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyTagStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, output.EntrySucceeded)
	assert.Equal(t, 1, output.TagSucceeded)
}

func TestApplyUseCase_Execute_TagFailure(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	strategy := newMockApplyTagStrategy()
	strategy.applyTagsErrors["/app/config"] = errors.New("tag api error")

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed 0 entries, 1 tags")
	assert.Equal(t, 0, output.TagSucceeded)
	assert.Equal(t, 1, output.TagFailed)
	require.Len(t, output.TagResults, 1)
	require.Error(t, output.TagResults[0].Error)

	// Failed tag should still be staged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	require.NoError(t, err)
}

func TestApplyUseCase_Execute_PartialTagFailure(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/success"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/fail"}, staging.TagEntry{
		Add:      map[string]string{"env": "dev"},
		StagedAt: time.Now(),
	}))

	strategy := newMockApplyTagStrategy()
	strategy.applyTagsErrors["/app/fail"] = errors.New("tag api error")

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed 0 entries, 1 tags")
	assert.Equal(t, 1, output.TagSucceeded)
	assert.Equal(t, 1, output.TagFailed)

	// Success should be unstaged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/success"})
	require.ErrorIs(t, err, staging.ErrNotStaged)

	// Failure should still be staged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/fail"})
	require.NoError(t, err)
}

func TestApplyUseCase_Execute_FilterByName_TagOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/one"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/two"}, staging.TagEntry{
		Add:      map[string]string{"env": "dev"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyTagStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		Name: "/app/one",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, output.TagSucceeded)
	require.Len(t, output.TagResults, 1)
	assert.Equal(t, "/app/one", output.TagResults[0].Name)

	// /app/two should still be staged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/two"})
	require.NoError(t, err)
}

func TestApplyUseCase_Execute_ListTagsError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListTagsErr = errors.New("list tags error")

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyTagStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list tags error")
}

// TestApplyUseCase_Execute_TagConflictDetection verifies that a staged tag whose
// remote was modified after its BaseModifiedAt is reported as a conflict and the
// apply is rejected (#483).
func TestApplyUseCase_Execute_TagConflictDetection(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	awsTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC) // modified after staging

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:            map[string]string{"env": "prod"},
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	strategy := newMockApplyTagStrategy()
	strategy.lastModified["/app/config"] = awsTime

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict")
	assert.Len(t, output.Conflicts, 1)
	assert.Equal(t, staging.EntryKey{Name: "/app/config"}, output.Conflicts[0])

	// The tag must NOT be applied or unstaged when a conflict is detected.
	assert.Equal(t, 0, output.TagSucceeded)

	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	require.NoError(t, err)
}

// TestApplyUseCase_Execute_TagConflictIgnored verifies that --ignore-conflicts
// bypasses tag conflict detection and applies the staged tag regardless (#483).
func TestApplyUseCase_Execute_TagConflictIgnored(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	awsTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC) // modified after staging

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:            map[string]string{"env": "prod"},
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	strategy := newMockApplyTagStrategy()
	strategy.lastModified["/app/config"] = awsTime

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Empty(t, output.Conflicts)
	assert.Equal(t, 1, output.TagSucceeded)

	// Applied cleanly, so the tag is unstaged.
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	require.ErrorIs(t, err, staging.ErrNotStaged)
}

// TestApplyUseCase_Execute_TagNoConflict verifies that a staged tag whose remote
// is unchanged since its BaseModifiedAt applies cleanly without --ignore-conflicts.
func TestApplyUseCase_Execute_TagNoConflict(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:            map[string]string{"env": "prod"},
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	strategy := newMockApplyTagStrategy()
	strategy.lastModified["/app/config"] = baseTime // unchanged since base

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{
		IgnoreConflicts: false,
	})
	require.NoError(t, err)
	assert.Empty(t, output.Conflicts)
	assert.Equal(t, 1, output.TagSucceeded)

	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	require.ErrorIs(t, err, staging.ErrNotStaged)
}

// TestApplyUseCase_Execute_PerNamespaceResolver covers the App Configuration
// path (#431): the same key staged under two namespaces lives in one store as
// two entries, each applied through the strategy resolved for its own namespace,
// and unstaged independently by (name, namespace).
func TestApplyUseCase_Execute_PerNamespaceResolver(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "app/k", Namespace: "dev"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("dev-val"), StagedAt: time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "app/k", Namespace: "prd"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("prd-val"), StagedAt: time.Now(),
	}))

	var mu sync.Mutex

	seen := map[string]bool{}
	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
		StrategyFor: func(namespace string) (staging.ApplyStrategy, error) {
			mu.Lock()
			seen[namespace] = true
			mu.Unlock()

			return newMockApplyStrategy(), nil
		},
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{IgnoreConflicts: true})
	require.NoError(t, err)
	assert.Equal(t, 2, output.EntrySucceeded)
	assert.True(t, seen["dev"] && seen["prd"], "the resolver must be called for each entry's namespace")

	// Each (name, namespace) is a distinct entry and is unstaged independently.
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "app/k", Namespace: "dev"})
	require.ErrorIs(t, err, staging.ErrNotStaged)
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "app/k", Namespace: "prd"})
	require.ErrorIs(t, err, staging.ErrNotStaged)

	// The applied namespace is reported on each result.
	got := []string{output.EntryResults[0].Namespace, output.EntryResults[1].Namespace}
	assert.ElementsMatch(t, []string{"dev", "prd"}, got)
}

// TestApplyUseCase_Execute_ResultOrder pins the result order to (name,
// namespace), so a name staged under several namespaces is reported in
// adjacent results, matching staging.SortedEntryKeys.
func TestApplyUseCase_Execute_ResultOrder(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	keys := []staging.EntryKey{{Name: "b"}, {Name: "a", Namespace: "z"}, {Name: "a"}}
	for _, key := range keys {
		require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, key, staging.Entry{
			Operation: staging.OperationCreate, Value: new("v"), StagedAt: time.Now(),
		}))
		require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, key, staging.TagEntry{
			Add: map[string]string{"env": "prod"}, StagedAt: time.Now(),
		}))
	}

	uc := &usecasestaging.ApplyUseCase{Strategy: newMockApplyStrategy(), Store: store}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{IgnoreConflicts: true})
	require.NoError(t, err)

	want := []staging.EntryKey{{Name: "a"}, {Name: "a", Namespace: "z"}, {Name: "b"}}
	assert.Equal(t, want, lo.Map(output.EntryResults, func(r usecasestaging.ApplyEntryResult, _ int) staging.EntryKey {
		return staging.EntryKey{Name: r.Name, Namespace: r.Namespace}
	}))
	assert.Equal(t, want, lo.Map(output.TagResults, func(r usecasestaging.ApplyTagResult, _ int) staging.EntryKey {
		return staging.EntryKey{Name: r.Name, Namespace: r.Namespace}
	}))
}

// TestApplyUseCase_Execute_ProbeErrorFailsClosed covers #989: when the conflict
// probe fails for a reason other than not-found, the key cannot be proven
// conflict-free, so the apply is rejected with the probe error and nothing is
// written. A not-found probe is still a definite answer and does not block.
func TestApplyUseCase_Execute_ProbeErrorFailsClosed(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, op := range []staging.Operation{staging.OperationCreate, staging.OperationUpdate, staging.OperationDelete} {
		t.Run(string(op), func(t *testing.T) {
			t.Parallel()

			key := staging.EntryKey{Name: "/app/probe"}
			store := testutil.NewMockStore()
			entry := staging.Entry{Operation: op, StagedAt: time.Now()}
			if op != staging.OperationDelete {
				entry.Value = new("staged")
			}
			if op != staging.OperationCreate {
				entry.BaseModifiedAt = &base
			}
			require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, key, entry))

			strategy := newMockApplyStrategy()
			strategy.fetchModifiedErr = errors.New("AccessDeniedException")

			uc := &usecasestaging.ApplyUseCase{Strategy: strategy, Store: store}

			output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
			require.EqualError(t, err, "apply rejected: conflict check failed (ignore conflicts to skip it): "+
				"cannot check /app/probe for conflicts: AccessDeniedException")
			assert.Nil(t, output)

			_, err = store.GetEntry(t.Context(), staging.ServiceParam, key)
			require.NoError(t, err, "the entry must stay staged")

			// A not-found probe does not block the apply.
			strategy.fetchModifiedErr = &staging.ResourceNotFoundError{}

			output, err = uc.Execute(t.Context(), usecasestaging.ApplyInput{})
			require.NoError(t, err)
			assert.Equal(t, 1, output.EntrySucceeded)
		})
	}
}
