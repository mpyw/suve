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

// newMockEditStrategyNotFound creates a mock that returns ResourceNotFoundError.
func newMockEditStrategyNotFound() *mockEditStrategy {
	return &mockEditStrategy{
		mockParser: newMockParser(),
		fetchErr:   &staging.ResourceNotFoundError{Err: errors.New("resource not found")},
	}
}

func TestEditUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:        "/app/config",
		Value:       "updated-value",
		Description: "updated desc",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)

	// Verify staged
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Equal(t, "updated-value", lo.FromPtr(entry.Value))
	assert.NotNil(t, entry.BaseModifiedAt)
}

func TestEditUseCase_Execute_PreservesBaseModifiedAt(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Pre-stage an entry
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
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
	_, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "newer-value",
	})
	require.NoError(t, err)

	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, baseTime, *entry.BaseModifiedAt)
}

func TestEditUseCase_Baseline_FromAWS(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-current-value",
		LastModified: time.Now(),
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Baseline(t.Context(), usecasestaging.BaselineInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Equal(t, "aws-current-value", output.Value)
}

func TestEditUseCase_Baseline_FromStaging(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	output, err := uc.Baseline(t.Context(), usecasestaging.BaselineInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Equal(t, "staged-value", output.Value)
}

func TestEditUseCase_Baseline_FromStagingCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("create-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	output, err := uc.Baseline(t.Context(), usecasestaging.BaselineInput{Name: "/app/new"})
	require.NoError(t, err)
	assert.Equal(t, "create-value", output.Value)
}

func TestEditUseCase_Execute_FetchError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockEditStrategy()
	strategy.fetchErr = errors.New("aws error")

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "new-value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aws error")
}

func TestEditUseCase_Baseline_FetchError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockEditStrategy()
	strategy.fetchErr = errors.New("aws error")

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Baseline(t.Context(), usecasestaging.BaselineInput{Name: "/app/config"})
	assert.Error(t, err)
}

func TestEditUseCase_Execute_WithStagedCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Pre-stage a create operation (no BaseModifiedAt)
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("initial"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/new",
		Value: "updated",
	})
	require.NoError(t, err)

	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
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

	store := testutil.NewMockStore()
	strategy := newMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Time{}, // Zero time
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "new-value",
	})
	require.NoError(t, err)

	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Nil(t, entry.BaseModifiedAt)
}

func TestEditUseCase_Execute_StageError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.StageEntryErr = errors.New("stage error")

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestEditUseCase_Execute_GetErrorForBaseModified(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetEntryErr = errors.New("get error")

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
}

func TestEditUseCase_Baseline_GetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetEntryErr = errors.New("get error")

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	_, err := uc.Baseline(t.Context(), usecasestaging.BaselineInput{Name: "/app/config"})
	assert.Error(t, err)
}

func TestEditUseCase_Execute_BlocksEditOnDelete(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Pre-stage a DELETE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/deleted", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.EditUseCase{
		Strategy: newMockEditStrategy(),
		Store:    store,
	}

	// Editing a staged DELETE should be blocked
	_, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/deleted",
		Value: "new-value",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staged for deletion")

	// Verify the operation is still DELETE
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/deleted")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
}

func TestEditUseCase_Baseline_BlocksWhenDeleteStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Pre-stage a DELETE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/deleted", staging.Entry{
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

	// When DELETE is staged, Baseline should return an error
	_, err := uc.Baseline(t.Context(), usecasestaging.BaselineInput{Name: "/app/deleted"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staged for deletion")
}

func TestEditUseCase_Execute_PreservesUpdateOperation(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Pre-stage an UPDATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
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
	_, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "newer-value",
	})
	require.NoError(t, err)

	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Equal(t, "newer-value", lo.FromPtr(entry.Value))
	assert.Equal(t, baseTime, *entry.BaseModifiedAt)
}

func TestEditUseCase_Execute_Skipped_SameAsAWS(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Now(),
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Edit with same value as AWS - should be skipped
	output, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "aws-value", // Same as AWS
	})
	require.NoError(t, err)
	assert.True(t, output.Skipped)
	assert.False(t, output.Unstaged)

	// Verify nothing was staged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestEditUseCase_Execute_Unstaged_RevertedToAWS(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Pre-stage an UPDATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-value"),
		StagedAt:  time.Now(),
	}))

	strategy := newMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Now(),
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Edit back to AWS value - should auto-unstage
	output, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "aws-value", // Reverted to AWS value
	})
	require.NoError(t, err)
	assert.False(t, output.Skipped)
	assert.True(t, output.Unstaged)

	// Verify entry was unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestEditUseCase_Execute_NotSkipped_DifferentFromAWS(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Now(),
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Edit with different value - should be staged
	output, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "new-value", // Different from AWS
	})
	require.NoError(t, err)
	assert.False(t, output.Skipped)
	assert.False(t, output.Unstaged)

	// Verify entry was staged
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
}

func TestEditUseCase_Execute_EmptyStringValue_AutoSkip(t *testing.T) {
	t.Parallel()

	// Test that empty string is treated as a valid AWS value (not "non-existing")
	// This is critical for Secrets Manager where empty string secrets are valid
	store := testutil.NewMockStore()
	strategy := &mockEditStrategy{
		mockParser: newMockParser(),
		fetchResult: &staging.EditFetchResult{
			Value:        "", // AWS has empty string
			LastModified: time.Now(),
		},
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Edit with empty string (same as AWS) - should auto-skip
	output, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "", // Same as AWS empty string
	})
	require.NoError(t, err)
	assert.True(t, output.Skipped, "should auto-skip when editing to same empty string")

	// Verify nothing was staged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestEditUseCase_Execute_EmptyStringValue_Stage(t *testing.T) {
	t.Parallel()

	// Test that we can stage a non-empty value when AWS has empty string
	store := testutil.NewMockStore()
	strategy := &mockEditStrategy{
		mockParser: newMockParser(),
		fetchResult: &staging.EditFetchResult{
			Value:        "", // AWS has empty string
			LastModified: time.Now(),
		},
	}

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Edit to non-empty value - should stage
	output, err := uc.Execute(t.Context(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "new-value",
	})
	require.NoError(t, err)
	assert.False(t, output.Skipped)

	// Verify entry was staged
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
}
