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
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
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
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/new", staging.Entry{
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
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/delete", staging.Entry{
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
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/same", staging.Entry{
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
	_, err = store.Get(staging.ServiceParam, "/app/same")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestDiffUseCase_Execute_AutoUnstage_AlreadyDeleted(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/gone", staging.Entry{
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
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/one", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("one"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/two", staging.Entry{
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
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/gone", staging.Entry{
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
	_, err = store.Get(staging.ServiceParam, "/app/gone")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestDiffUseCase_Execute_WithTags(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/tagged", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value"),
		Tags:      map[string]string{"env": "test"},
		StagedAt:  time.Now(),
	}))

	strategy := newMockDiffStrategy()
	strategy.fetchErrors["/app/tagged"] = errors.New("not found")

	uc := &usecasestaging.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DiffInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)
	assert.Equal(t, "test", output.Entries[0].Tags["env"])
}
