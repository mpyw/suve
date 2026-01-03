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

type mockEditStrategy struct {
	*mockParser
	fetchResult *staging.EditFetchResult
	fetchErr    error
}

func (m *mockEditStrategy) FetchCurrentValue(_ context.Context, _ string) (*staging.EditFetchResult, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	return m.fetchResult, nil
}

func newMockEditStrategy() *mockEditStrategy {
	return &mockEditStrategy{
		mockParser: newMockParser(),
		fetchResult: &staging.EditFetchResult{
			Value:        "aws-value",
			LastModified: time.Now(),
		},
	}
}

func TestEditUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:        "/app/config",
		Value:       "updated-value",
		Description: "updated desc",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)

	// Verify staged
	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Equal(t, "updated-value", lo.FromPtr(entry.Value))
	assert.NotNil(t, entry.BaseModifiedAt)
}

func TestEditUseCase_Execute_PreservesBaseModifiedAt(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Pre-stage an entry
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("old-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	// Re-edit should preserve original BaseModifiedAt
	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "newer-value",
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, baseTime, *entry.BaseModifiedAt)
}

func TestEditUseCase_Baseline_FromAWS(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-current-value",
		LastModified: time.Now(),
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Baseline(context.Background(), usecasestaging.BaselineInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Equal(t, "aws-current-value", output.Value)
}

func TestEditUseCase_Baseline_FromStaging(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	output, err := uc.Baseline(context.Background(), usecasestaging.BaselineInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Equal(t, "staged-value", output.Value)
}

func TestEditUseCase_Baseline_FromStagingCreate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("create-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	output, err := uc.Baseline(context.Background(), usecasestaging.BaselineInput{Name: "/app/new"})
	require.NoError(t, err)
	assert.Equal(t, "create-value", output.Value)
}

func TestEditUseCase_Execute_FetchError(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockEditStrategy()
	strategy.fetchErr = errors.New("aws error")

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "new-value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aws error")
}

func TestEditUseCase_Baseline_FetchError(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockEditStrategy()
	strategy.fetchErr = errors.New("aws error")

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Baseline(context.Background(), usecasestaging.BaselineInput{Name: "/app/config"})
	assert.Error(t, err)
}

func TestEditUseCase_Execute_WithStagedCreate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Pre-stage a create operation (no BaseModifiedAt)
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("initial"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/new",
		Value: "updated",
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/new")
	require.NoError(t, err)
	// Operation should remain Create (not become Update)
	assert.Equal(t, staging.OperationCreate, entry.Operation)
	// BaseModifiedAt should remain nil for create operations
	assert.Nil(t, entry.BaseModifiedAt)
	// Value should be updated
	assert.Equal(t, "updated", lo.FromPtr(entry.Value))
}

func TestEditUseCase_Execute_ZeroLastModified(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Time{}, // Zero time
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "new-value",
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Nil(t, entry.BaseModifiedAt)
}

func TestEditUseCase_Execute_StageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.stageErr = errors.New("stage error")

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestEditUseCase_Execute_GetErrorForBaseModified(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("get error")

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
}

func TestEditUseCase_Baseline_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("get error")

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	_, err := uc.Baseline(context.Background(), usecasestaging.BaselineInput{Name: "/app/config"})
	assert.Error(t, err)
}

func TestEditUseCase_Execute_ConvertsDeleteToUpdate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Pre-stage a DELETE operation
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/deleted", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	// Editing a staged DELETE should convert it to UPDATE
	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/deleted",
		Value: "new-value",
	})
	require.NoError(t, err)

	// Verify the operation changed to UPDATE
	entry, err := store.Get(staging.ServiceParam, "/app/deleted")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
}

func TestEditUseCase_Baseline_FetchesFromAWSWhenDeleteStaged(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Pre-stage a DELETE operation
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/deleted", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	strategy := newMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-current-value",
		LastModified: time.Now(),
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// When DELETE is staged, Baseline should fetch from AWS
	output, err := uc.Baseline(context.Background(), usecasestaging.BaselineInput{Name: "/app/deleted"})
	require.NoError(t, err)
	assert.Equal(t, "aws-current-value", output.Value)
}

func TestEditUseCase_Execute_PreservesUpdateOperation(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Pre-stage an UPDATE operation
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("old-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	// Re-edit should preserve UPDATE operation
	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "newer-value",
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Equal(t, "newer-value", lo.FromPtr(entry.Value))
	assert.Equal(t, baseTime, *entry.BaseModifiedAt)
}
