package staging_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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
	entry, err := store.Get(staging.ServiceParam, "/app/to-delete")
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

	entry, err := store.Get(staging.ServiceSecret, "my-secret")
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
