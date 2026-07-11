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
	"github.com/mpyw/suve/internal/staging/store/testutil"
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

	store := testutil.NewMockStore()
	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/to-delete"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/to-delete", output.Name)
	assert.False(t, output.ShowDeleteOptions)

	// Verify staged
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/to-delete", Namespace: ""})
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
	assert.NotNil(t, entry.BaseModifiedAt)
}

func TestDeleteUseCase_Execute_SecretWithRecoveryWindow(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockDeleteStrategy(true)
	strategy.mockServiceStrategy = newSecretStrategy()

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key:            staging.EntryKey{Name: "my-secret"},
		RecoveryWindow: 14,
	})
	require.NoError(t, err)
	assert.True(t, output.ShowDeleteOptions)
	assert.Equal(t, 14, output.RecoveryWindow)
	assert.False(t, output.Force)

	entry, err := store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret", Namespace: ""})
	require.NoError(t, err)
	assert.NotNil(t, entry.DeleteOptions)
	assert.Equal(t, 14, entry.DeleteOptions.RecoveryWindow)
}

func TestDeleteUseCase_Execute_SecretForceDelete(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockDeleteStrategy(true)
	strategy.mockServiceStrategy = newSecretStrategy()

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key:   staging.EntryKey{Name: "my-secret"},
		Force: true,
	})
	require.NoError(t, err)
	assert.True(t, output.Force)
}

func TestDeleteUseCase_Execute_InvalidRecoveryWindow(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockDeleteStrategy(true)
	strategy.mockServiceStrategy = newSecretStrategy()

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Too short
	_, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key:            staging.EntryKey{Name: "my-secret"},
		RecoveryWindow: 5,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recovery window")

	// Too long
	_, err = uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key:            staging.EntryKey{Name: "my-secret"},
		RecoveryWindow: 31,
	})
	assert.Error(t, err)
}

func TestDeleteUseCase_Execute_FetchError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockDeleteStrategy(false)
	// A generic (non not-found) fetch failure must surface as an error.
	strategy.fetchErr = errors.New("access denied")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/config"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch")
}

func TestDeleteUseCase_Execute_ResourceNotFound(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockDeleteStrategy(false)
	// The strategy signals a genuine not-found via *ResourceNotFoundError.
	strategy.fetchErr = &staging.ResourceNotFoundError{}

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Delete should fail when the resource doesn't exist and is not staged.
	_, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/not-exists"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource not found")
}

func TestDeleteUseCase_Execute_StagedUpdate_RemoteVanished(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// A value was staged as an Update, then the remote was deleted out-of-band.
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/gone"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("v2"),
		StagedAt:  time.Now(),
	}))

	strategy := newMockDeleteStrategy(false)
	strategy.fetchErr = &staging.ResourceNotFoundError{} // Remote no longer exists

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Delete must not dead-end silently: it errors and names reset as the remedy.
	_, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/gone"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reset")

	// The staged Update must be left intact so reset can still discard it.
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/gone", Namespace: ""})
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
}

func TestDeleteUseCase_Execute_ZeroLastModified_ResourceExists(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	strategy := newMockDeleteStrategy(false)
	// The resource EXISTS but reports a zero modification time (nil error). This
	// must NOT be misclassified as "not found" (#334): it can still be staged
	// for deletion.
	strategy.lastModified = time.Time{}

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/to-delete"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/to-delete", output.Name)
	assert.False(t, output.Unstaged)

	// Verify it was staged for deletion, with no conflict base (zero Modified).
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/to-delete", Namespace: ""})
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
	assert.Nil(t, entry.BaseModifiedAt)
}

func TestDeleteUseCase_Execute_ZeroLastModified_StagedCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Pre-stage a CREATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new-param"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))

	strategy := newMockDeleteStrategy(false)
	strategy.fetchErr = &staging.ResourceNotFoundError{} // Resource doesn't exist on AWS

	uc := &usecasestaging.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Delete should succeed by unstaging the CREATE
	output, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/new-param"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new-param", output.Name)
	assert.True(t, output.Unstaged) // Should be unstaged, not deleted

	// Verify entry is removed
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new-param", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestDeleteUseCase_Execute_StageError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.StageEntryErr = errors.New("stage error")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/config"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestDeleteUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetEntryErr = errors.New("store get error")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/config"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store get error")
}

func TestDeleteUseCase_Execute_UnstageError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Simulate existing CREATE entry by staging it
	store.AddEntry(staging.ServiceParam, staging.EntryKey{Name: "/app/new"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value"),
	})

	store.UnstageEntryErr = errors.New("unstage error")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/new"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unstage error")
}

func TestDeleteUseCase_Execute_UnstagesCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Pre-stage a CREATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/new"},
	})
	require.NoError(t, err)
	assert.True(t, output.Unstaged)
	assert.Equal(t, "/app/new", output.Name)

	// Verify the entry was unstaged (removed), not staged as DELETE
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestDeleteUseCase_Execute_DeleteOnUpdate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Pre-stage an UPDATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/existing"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("updated-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/existing"},
	})
	require.NoError(t, err)
	assert.False(t, output.Unstaged) // Not unstaged, it was re-staged as DELETE

	// Verify the operation changed to DELETE
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/existing", Namespace: ""})
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
}

func TestDeleteUseCase_Execute_ExistingWithStagedTags_UnstagesTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// A tag was staged against an existing resource (entry NotStaged, tag staged).
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/to-delete"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	// Deleting the resource must stage a DELETE and discard the orphan tags so
	// they don't fail against a resource that no longer exists (#470).
	output, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/to-delete"},
	})
	require.NoError(t, err)
	assert.False(t, output.Unstaged)

	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/to-delete", Namespace: ""})
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)

	// Tags must be gone.
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/to-delete", Namespace: ""})
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestDeleteUseCase_Execute_UnstageTagError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Simulate existing CREATE entry
	store.AddEntry(staging.ServiceParam, staging.EntryKey{Name: "/app/new"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value"),
	})

	// Simulate existing tag entry
	store.AddTag(staging.ServiceParam, staging.EntryKey{Name: "/app/new"}, staging.TagEntry{
		Add: map[string]string{"env": "prod"},
	})

	// Make UnstageTag fail
	store.UnstageTagErr = errors.New("unstage tag error")

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.DeleteInput{
		Key: staging.EntryKey{Name: "/app/new"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unstage tag error")
}
