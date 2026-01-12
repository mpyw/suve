package staging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/testutil"
	usecasestaging "github.com/mpyw/suve/internal/usecase/staging"
)

type mockApplyStrategy struct {
	*mockServiceStrategy
	applyErrors      map[string]error
	lastModified     map[string]time.Time
	fetchModifiedErr error
}

func (m *mockApplyStrategy) Apply(_ context.Context, name string, _ staging.Entry) error {
	if err, ok := m.applyErrors[name]; ok {
		return err
	}
	return nil
}

func (m *mockApplyStrategy) ApplyTags(_ context.Context, _ string, _ staging.TagEntry) error {
	return nil
}

func (m *mockApplyStrategy) FetchLastModified(_ context.Context, name string) (time.Time, error) {
	if m.fetchModifiedErr != nil {
		return time.Time{}, m.fetchModifiedErr
	}
	if t, ok := m.lastModified[name]; ok {
		return t, nil
	}
	return time.Now(), nil
}

func newMockApplyStrategy() *mockApplyStrategy {
	return &mockApplyStrategy{
		mockServiceStrategy: newParamStrategy(),
		applyErrors:         make(map[string]error),
		lastModified:        make(map[string]time.Time),
	}
}

func TestApplyUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	require.NoError(t, err)
	assert.Equal(t, "SSM Parameter Store", output.ServiceName)
	assert.Equal(t, "parameter", output.ItemName)
	assert.Empty(t, output.EntryResults)
	assert.Equal(t, 0, output.EntrySucceeded)
	assert.Equal(t, 0, output.EntryFailed)
}

func TestApplyUseCase_Execute_SingleCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
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
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_MultipleOperations(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/create", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("create"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/update", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("update"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/delete", staging.Entry{
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

func TestApplyUseCase_Execute_FilterByName(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/one", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("one"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/two", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("two"),
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
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/two")
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not staged")
}

func TestApplyUseCase_Execute_PartialFailure(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/success", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("success"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/fail", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("fail"),
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "applied 1 entries")
	assert.Equal(t, 1, output.EntrySucceeded)
	assert.Equal(t, 1, output.EntryFailed)

	// Failed entry should still be staged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/fail")
	require.NoError(t, err)

	// Successful entry should be unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/success")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_ConflictDetection(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	awsTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC) // Modified after staging

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/conflict", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("staged"),
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conflict")
	assert.Len(t, output.Conflicts, 1)
	assert.Equal(t, "/app/conflict", output.Conflicts[0])
}

func TestApplyUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.listErr = errors.New("list error")

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

func TestApplyUseCase_Execute_DeleteSuccess(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/to-delete", staging.Entry{
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
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
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
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_TagsWithRemove(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	removeKeys := map[string]struct{}{"deprecated": {}, "old": {}}
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
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
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
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
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed 0 entries, 1 tags")
	assert.Equal(t, 0, output.TagSucceeded)
	assert.Equal(t, 1, output.TagFailed)
	require.Len(t, output.TagResults, 1)
	assert.Error(t, output.TagResults[0].Error)

	// Failed tag should still be staged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
}

func TestApplyUseCase_Execute_PartialTagFailure(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/success", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/fail", staging.TagEntry{
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed 0 entries, 1 tags")
	assert.Equal(t, 1, output.TagSucceeded)
	assert.Equal(t, 1, output.TagFailed)

	// Success should be unstaged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/success")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	// Failure should still be staged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/fail")
	require.NoError(t, err)
}

func TestApplyUseCase_Execute_FilterByName_TagOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/one", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/two", staging.TagEntry{
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
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/two")
	require.NoError(t, err)
}

func TestApplyUseCase_Execute_ListTagsError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.listTagsErr = errors.New("list tags error")

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyTagStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ApplyInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list tags error")
}
