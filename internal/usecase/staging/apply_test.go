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

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{})
	require.NoError(t, err)
	assert.Equal(t, "SSM Parameter Store", output.ServiceName)
	assert.Equal(t, "parameter", output.ItemName)
	assert.Empty(t, output.Results)
	assert.Equal(t, 0, output.Succeeded)
	assert.Equal(t, 0, output.Failed)
}

func TestApplyUseCase_Execute_SingleCreate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, output.Succeeded)
	assert.Equal(t, 0, output.Failed)
	require.Len(t, output.Results, 1)
	assert.Equal(t, usecasestaging.ApplyResultCreated, output.Results[0].Status)

	// Verify unstaged after apply
	_, err = store.Get(staging.ServiceParam, "/app/new")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_MultipleOperations(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/create", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("create"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/update", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("update"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/delete", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, output.Succeeded)
	assert.Equal(t, 0, output.Failed)
}

func TestApplyUseCase_Execute_FilterByName(t *testing.T) {
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

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		Name:            "/app/one",
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, output.Succeeded)
	require.Len(t, output.Results, 1)
	assert.Equal(t, "/app/one", output.Results[0].Name)

	// /app/two should still be staged
	_, err = store.Get(staging.ServiceParam, "/app/two")
	require.NoError(t, err)
}

func TestApplyUseCase_Execute_FilterByName_NotStaged(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		Name: "/app/not-staged",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not staged")
}

func TestApplyUseCase_Execute_PartialFailure(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/success", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("success"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/fail", staging.Entry{
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

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "applied 1, failed 1")
	assert.Equal(t, 1, output.Succeeded)
	assert.Equal(t, 1, output.Failed)

	// Failed entry should still be staged
	_, err = store.Get(staging.ServiceParam, "/app/fail")
	require.NoError(t, err)

	// Successful entry should be unstaged
	_, err = store.Get(staging.ServiceParam, "/app/success")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_ConflictDetection(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	awsTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC) // Modified after staging

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/conflict", staging.Entry{
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

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
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

	_, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

func TestApplyUseCase_Execute_DeleteSuccess(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/to-delete", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, output.Succeeded)
	require.Len(t, output.Results, 1)
	assert.Equal(t, usecasestaging.ApplyResultDeleted, output.Results[0].Status)
}
