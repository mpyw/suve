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

type mockDeleteStrategy struct {
	*mockServiceStrategy
	lastModified time.Time
	fetchErr     error
}

func (m *mockDeleteStrategy) FetchLastModified(_ context.Context, _ string) (time.Time, error) {
	if m.fetchErr != nil {
		return time.Time{}, m.fetchErr
	}
	return m.lastModified, nil
}

func newMockDeleteStrategy(hasDeleteOptions bool) *mockDeleteStrategy {
	strategy := &mockDeleteStrategy{
		mockServiceStrategy: newParamStrategy(),
		lastModified:        time.Now(),
	}
	strategy.hasDeleteOptions = hasDeleteOptions
	return strategy
}

func TestDeleteUseCase_Execute_Param(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/to-delete",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/to-delete", output.Name)
	assert.False(t, output.HasDeleteOptions)

	// Verify staged
	entry, err := store.GetEntry(staging.ServiceParam, "/app/to-delete")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
	assert.NotNil(t, entry.BaseModifiedAt)
}

func TestDeleteUseCase_Execute_SecretWithRecoveryWindow(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockDeleteStrategy(true)
	strategy.mockServiceStrategy = newSecretStrategy()

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name:           "my-secret",
		RecoveryWindow: 14,
	})
	require.NoError(t, err)
	assert.True(t, output.HasDeleteOptions)
	assert.Equal(t, 14, output.RecoveryWindow)
	assert.False(t, output.Force)

	entry, err := store.GetEntry(staging.ServiceSecret, "my-secret")
	require.NoError(t, err)
	assert.NotNil(t, entry.DeleteOptions)
	assert.Equal(t, 14, entry.DeleteOptions.RecoveryWindow)
}

func TestDeleteUseCase_Execute_SecretForceDelete(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockDeleteStrategy(true)
	strategy.mockServiceStrategy = newSecretStrategy()

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name:  "my-secret",
		Force: true,
	})
	require.NoError(t, err)
	assert.True(t, output.Force)
}

func TestDeleteUseCase_Execute_InvalidRecoveryWindow(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockDeleteStrategy(true)
	strategy.mockServiceStrategy = newSecretStrategy()

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Too short
	_, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name:           "my-secret",
		RecoveryWindow: 5,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "recovery window")

	// Too long
	_, err = uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name:           "my-secret",
		RecoveryWindow: 31,
	})
	assert.Error(t, err)
}

func TestDeleteUseCase_Execute_FetchError(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockDeleteStrategy(false)
	strategy.fetchErr = errors.New("not found")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/not-exists",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch")
}

func TestDeleteUseCase_Execute_ZeroLastModified(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockDeleteStrategy(false)
	strategy.lastModified = time.Time{} // Zero time

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/to-delete",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/to-delete", output.Name)

	// Verify BaseModifiedAt is nil when lastModified is zero
	entry, err := store.GetEntry(staging.ServiceParam, "/app/to-delete")
	require.NoError(t, err)
	assert.Nil(t, entry.BaseModifiedAt)
}

func TestDeleteUseCase_Execute_StageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.stageErr = errors.New("stage error")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/config",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestDeleteUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("store get error")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/config",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store get error")
}

func TestDeleteUseCase_Execute_UnstageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	// Simulate existing CREATE entry by staging it
	store.entries[staging.ServiceParam]["/app/new"] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value"),
	}
	store.unstageErr = errors.New("unstage error")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/new",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unstage error")
}

func TestDeleteUseCase_Execute_UnstagesCreate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Pre-stage a CREATE operation
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/new",
	})
	require.NoError(t, err)
	assert.True(t, output.Unstaged)
	assert.Equal(t, "/app/new", output.Name)

	// Verify the entry was unstaged (removed), not staged as DELETE
	_, err = store.GetEntry(staging.ServiceParam, "/app/new")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestDeleteUseCase_Execute_DeleteOnUpdate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Pre-stage an UPDATE operation
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/existing", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("updated-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/existing",
	})
	require.NoError(t, err)
	assert.False(t, output.Unstaged) // Not unstaged, it was re-staged as DELETE

	// Verify the operation changed to DELETE
	entry, err := store.GetEntry(staging.ServiceParam, "/app/existing")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
}
