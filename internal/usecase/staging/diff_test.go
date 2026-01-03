package staging_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	usecasestaging "github.com/mpyw/suve/internal/usecase/staging"
)

type mockDiffStrategy struct {
	*mockServiceStrategy
	fetchResults map[string]*staging.FetchResult
	fetchErrors  map[string]error
}

func (m *mockDiffStrategy) FetchCurrent(_ context.Context, name string) (*staging.FetchResult, error) {
	if err, ok := m.fetchErrors[name]; ok {
		return nil, err
	}
	if result, ok := m.fetchResults[name]; ok {
		return result, nil
	}
	return &staging.FetchResult{Value: "aws-value", Identifier: "#1"}, nil
}

func newMockDiffStrategy() *mockDiffStrategy {
	return &mockDiffStrategy{
		mockServiceStrategy: newParamStrategy(),
		fetchResults:        make(map[string]*staging.FetchResult),
		fetchErrors:         make(map[string]error),
	}
}

func TestDiffUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.DiffUseCase{
		Strategy: newMockDiffStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	assert.Equal(t, "parameter", output.ItemName)
	assert.Empty(t, output.Entries)
}

func TestDiffUseCase_Execute_UpdateDiff(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))

	strategy := newMockDiffStrategy()
	strategy.fetchResults["/app/config"] = &staging.FetchResult{
		Value:      "old-value",
		Identifier: "#5",
	}

	uc := &usecasestaging.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)

	entry := output.Entries[0]
	assert.Equal(t, "/app/config", entry.Name)
	assert.Equal(t, usecasestaging.DiffEntryNormal, entry.Type)
	assert.Equal(t, "old-value", entry.AWSValue)
	assert.Equal(t, "new-value", entry.StagedValue)
	assert.Equal(t, "#5", entry.AWSIdentifier)
}

func TestDiffUseCase_Execute_CreateDiff(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/new", staging.Entry{
		Operation:   staging.OperationCreate,
		Value:       lo.ToPtr("new-value"),
		Description: lo.ToPtr("new param"),
		StagedAt:    time.Now(),
	}))

	strategy := newMockDiffStrategy()
	strategy.fetchErrors["/app/new"] = errors.New("not found")

	uc := &usecasestaging.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)

	entry := output.Entries[0]
	assert.Equal(t, usecasestaging.DiffEntryCreate, entry.Type)
	assert.Equal(t, "new-value", entry.StagedValue)
	assert.Equal(t, "new param", *entry.Description)
}

func TestDiffUseCase_Execute_DeleteDiff(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/delete", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	strategy := newMockDiffStrategy()
	strategy.fetchResults["/app/delete"] = &staging.FetchResult{
		Value:      "existing-value",
		Identifier: "#10",
	}

	uc := &usecasestaging.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)

	entry := output.Entries[0]
	assert.Equal(t, usecasestaging.DiffEntryNormal, entry.Type)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
	assert.Equal(t, "existing-value", entry.AWSValue)
	assert.Empty(t, entry.StagedValue) // Delete has no staged value
}

func TestDiffUseCase_Execute_AutoUnstage_Identical(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/same", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("same-value"),
		StagedAt:  time.Now(),
	}))

	strategy := newMockDiffStrategy()
	strategy.fetchResults["/app/same"] = &staging.FetchResult{
		Value:      "same-value", // Same as staged
		Identifier: "#1",
	}

	uc := &usecasestaging.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)

	entry := output.Entries[0]
	assert.Equal(t, usecasestaging.DiffEntryAutoUnstaged, entry.Type)
	assert.Contains(t, entry.Warning, "identical")

	// Verify auto-unstaged
	_, err = store.GetEntry(staging.ServiceParam, "/app/same")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestDiffUseCase_Execute_AutoUnstage_AlreadyDeleted(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/gone", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	strategy := newMockDiffStrategy()
	strategy.fetchErrors["/app/gone"] = errors.New("not found")

	uc := &usecasestaging.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)

	entry := output.Entries[0]
	assert.Equal(t, usecasestaging.DiffEntryAutoUnstaged, entry.Type)
	assert.Contains(t, entry.Warning, "already deleted")
}

func TestDiffUseCase_Execute_FilterByName(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/one", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("one"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/two", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("two"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.DiffUseCase{
		Strategy: newMockDiffStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{Name: "/app/one"})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)
	assert.Equal(t, "/app/one", output.Entries[0].Name)
}

func TestDiffUseCase_Execute_FilterByName_NotStaged(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.DiffUseCase{
		Strategy: newMockDiffStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{Name: "/app/not-staged"})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)
	assert.Equal(t, usecasestaging.DiffEntryWarning, output.Entries[0].Type)
	assert.Contains(t, output.Entries[0].Warning, "not staged")
}

func TestDiffUseCase_Execute_AutoUnstage_UpdateNoLongerExists(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Stage an update for something that no longer exists
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/gone", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("update-value"),
		StagedAt:  time.Now(),
	}))

	strategy := newMockDiffStrategy()
	strategy.fetchErrors["/app/gone"] = errors.New("not found")

	uc := &usecasestaging.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)

	entry := output.Entries[0]
	assert.Equal(t, usecasestaging.DiffEntryAutoUnstaged, entry.Type)
	assert.Contains(t, entry.Warning, "no longer exists")

	// Verify auto-unstaged
	_, err = store.GetEntry(staging.ServiceParam, "/app/gone")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestDiffUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.listErr = errors.New("list error")

	uc := &usecasestaging.DiffUseCase{
		Strategy: newMockDiffStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

func TestDiffUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("get error")

	uc := &usecasestaging.DiffUseCase{
		Strategy: newMockDiffStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.DiffInput{Name: "/app/config"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get error")
}

func TestDiffUseCase_Execute_GetTagError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getTagErr = errors.New("get tag error")

	uc := &usecasestaging.DiffUseCase{
		Strategy: newMockDiffStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.DiffInput{Name: "/app/config"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get tag error")
}

func TestDiffUseCase_Execute_ListTagsError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.listTagsErr = errors.New("list tags error")

	uc := &usecasestaging.DiffUseCase{
		Strategy: newMockDiffStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list tags error")
}

func TestDiffUseCase_Execute_UnknownOperation(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Stage an entry with an unknown operation (edge case)
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/unknown", staging.Entry{
		Operation: staging.Operation("unknown"), // Invalid operation
		Value:     lo.ToPtr("value"),
		StagedAt:  time.Now(),
	}))

	strategy := newMockDiffStrategy()
	strategy.fetchErrors["/app/unknown"] = errors.New("fetch error")

	uc := &usecasestaging.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)
	assert.Equal(t, usecasestaging.DiffEntryWarning, output.Entries[0].Type)
	assert.Contains(t, output.Entries[0].Warning, "fetch error")
}
