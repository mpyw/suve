package transition

import (
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/testutil"
)

func TestNewExecutor(t *testing.T) {
	store := testutil.NewMockStore()
	executor := NewExecutor(store)
	assert.NotNil(t, executor)
	assert.Equal(t, store, executor.Store)
}

func TestExecuteEntry_Add(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: nil,
		StagedState:  EntryStagedStateNotStaged{},
	}

	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/new", state, EntryActionAdd{Value: "new-value"}, nil)
	require.NoError(t, err)

	// Check result
	_, isCreate := result.NewState.StagedState.(EntryStagedStateCreate)
	assert.True(t, isCreate)

	// Check persisted
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationCreate, entry.Operation)
	assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
}

func TestExecuteEntry_AddWithDescription(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: nil,
		StagedState:  EntryStagedStateNotStaged{},
	}

	description := "test description"
	opts := &EntryExecuteOptions{Description: &description}
	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/new", state, EntryActionAdd{Value: "new-value"}, opts)
	require.NoError(t, err)

	_, isCreate := result.NewState.StagedState.(EntryStagedStateCreate)
	assert.True(t, isCreate)

	// Check persisted with description
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationCreate, entry.Operation)
	assert.Equal(t, "test description", lo.FromPtr(entry.Description))
}

func TestExecuteEntry_Edit(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	currentValue := "current"
	state := EntryState{
		CurrentValue: &currentValue,
		StagedState:  EntryStagedStateNotStaged{},
	}

	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/config", state, EntryActionEdit{Value: "updated"}, nil)
	require.NoError(t, err)

	// Check result
	_, isUpdate := result.NewState.StagedState.(EntryStagedStateUpdate)
	assert.True(t, isUpdate)

	// Check persisted
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
}

func TestExecuteEntry_EditWithMetadata(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	currentValue := "current"
	state := EntryState{
		CurrentValue: &currentValue,
		StagedState:  EntryStagedStateNotStaged{},
	}

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	description := "edit description"
	opts := &EntryExecuteOptions{
		BaseModifiedAt: &baseTime,
		Description:    &description,
	}

	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/config", state, EntryActionEdit{Value: "updated"}, opts)
	require.NoError(t, err)

	_, isUpdate := result.NewState.StagedState.(EntryStagedStateUpdate)
	assert.True(t, isUpdate)

	// Check persisted with metadata
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Equal(t, baseTime, *entry.BaseModifiedAt)
	assert.Equal(t, "edit description", lo.FromPtr(entry.Description))
}

func TestExecuteEntry_Delete(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	currentValue := "current"
	state := EntryState{
		CurrentValue: &currentValue,
		StagedState:  EntryStagedStateNotStaged{},
	}

	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/config", state, EntryActionDelete{}, nil)
	require.NoError(t, err)

	// Check result
	_, isDelete := result.NewState.StagedState.(EntryStagedStateDelete)
	assert.True(t, isDelete)

	// Check persisted
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
}

func TestExecuteEntry_DeleteWithMetadata(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	currentValue := "current"
	state := EntryState{
		CurrentValue: &currentValue,
		StagedState:  EntryStagedStateNotStaged{},
	}

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	opts := &EntryExecuteOptions{
		BaseModifiedAt: &baseTime,
	}

	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/config", state, EntryActionDelete{}, opts)
	require.NoError(t, err)

	_, isDelete := result.NewState.StagedState.(EntryStagedStateDelete)
	assert.True(t, isDelete)

	// Check persisted with metadata
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)
	assert.Equal(t, baseTime, *entry.BaseModifiedAt)
}

func TestExecuteEntry_DeleteCreate_UnstagesTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Pre-stage CREATE and tags
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("draft"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/new", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: nil,
		StagedState:  EntryStagedStateCreate{DraftValue: "draft"},
	}

	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/new", state, EntryActionDelete{}, nil)
	require.NoError(t, err)

	// Check DiscardTags flag
	assert.True(t, result.DiscardTags)

	// Check entry unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	// Check tags unstaged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/new")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestExecuteEntry_DeleteCreate_NoTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Pre-stage CREATE but no tags
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("draft"),
		StagedAt:  time.Now(),
	}))

	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: nil,
		StagedState:  EntryStagedStateCreate{DraftValue: "draft"},
	}

	// Delete CREATE without tags - should succeed (ErrNotStaged ignored)
	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/new", state, EntryActionDelete{}, nil)
	require.NoError(t, err)

	// Check DiscardTags flag is set
	assert.True(t, result.DiscardTags)

	// Check entry unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestExecuteEntry_Reset(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Pre-stage an entry
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("updated"),
		StagedAt:  time.Now(),
	}))

	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: lo.ToPtr("current"),
		StagedState:  EntryStagedStateUpdate{DraftValue: "updated"},
	}

	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/config", state, EntryActionReset{}, nil)
	require.NoError(t, err)

	// Check result
	_, isNotStaged := result.NewState.StagedState.(EntryStagedStateNotStaged)
	assert.True(t, isNotStaged)

	// Check unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestExecuteEntry_Error(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: lo.ToPtr("current"),
		StagedState:  EntryStagedStateDelete{},
	}

	// Edit on DELETE should error
	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/config", state, EntryActionEdit{Value: "new"}, nil)
	assert.ErrorIs(t, err, ErrCannotEditDelete)
	assert.Equal(t, ErrCannotEditDelete, result.Error)
}

func TestExecuteEntry_ResetNotStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: nil,
		StagedState:  EntryStagedStateNotStaged{},
	}

	// Reset on NotStaged should succeed (no-op)
	result, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/config", state, EntryActionReset{}, nil)
	require.NoError(t, err)

	_, isNotStaged := result.NewState.StagedState.(EntryStagedStateNotStaged)
	assert.True(t, isNotStaged)
}

func TestExecuteTag_Add(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	action := TagActionTag{
		Tags:           map[string]string{"env": "prod"},
		CurrentAWSTags: nil, // nil disables auto-skip
	}
	existingValue := "existing-value"
	entryState := EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}}

	result, err := executor.ExecuteTag(t.Context(), staging.ServiceParam, "/app/config", entryState, StagedTags{}, action, &baseTime)
	require.NoError(t, err)

	assert.Equal(t, "prod", result.NewStagedTags.ToSet["env"])

	// Check persisted
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
	assert.Equal(t, baseTime, *tagEntry.BaseModifiedAt)
}

func TestExecuteTag_Remove(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	action := TagActionUntag{
		Keys:              maputil.NewSet("deprecated"),
		CurrentAWSTagKeys: nil, // nil disables auto-skip
	}
	existingValue := "existing-value"
	entryState := EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}}

	result, err := executor.ExecuteTag(t.Context(), staging.ServiceParam, "/app/config", entryState, StagedTags{}, action, nil)
	require.NoError(t, err)

	assert.True(t, result.NewStagedTags.ToUnset.Contains("deprecated"))

	// Check persisted
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.True(t, tagEntry.Remove.Contains("deprecated"))
}

func TestExecuteTag_Error(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	action := TagActionTag{
		Tags: map[string]string{"env": "prod"},
	}
	existingValue := "existing-value"
	entryState := EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateDelete{}}

	// Tag on DELETE should error
	result, err := executor.ExecuteTag(t.Context(), staging.ServiceParam, "/app/config", entryState, StagedTags{}, action, nil)
	assert.ErrorIs(t, err, ErrCannotTagDelete)
	assert.Equal(t, ErrCannotTagDelete, result.Error)
}

func TestExecuteTag_UnstageWhenEmpty(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Pre-stage tags
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	executor := NewExecutor(store)

	// Remove the only tag that was set to add
	existingTags := StagedTags{
		ToSet: map[string]string{"env": "prod"},
	}
	action := TagActionUntag{
		Keys:              maputil.NewSet("env"),
		CurrentAWSTagKeys: maputil.NewSet("env"),
	}
	existingValue := "existing-value"
	entryState := EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}}

	result, err := executor.ExecuteTag(t.Context(), staging.ServiceParam, "/app/config", entryState, existingTags, action, nil)
	require.NoError(t, err)

	// Result should have only ToUnset, no ToSet
	assert.Empty(t, result.NewStagedTags.ToSet)
	assert.True(t, result.NewStagedTags.ToUnset.Contains("env"))

	// But since it's not empty (has ToUnset), it should remain staged
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.True(t, tagEntry.Remove.Contains("env"))
}

func TestExecuteTag_UnstageWhenCompletelyEmpty(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Pre-stage tags to add "env": "prod"
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	executor := NewExecutor(store)

	// Staged to add "env": "prod", but AWS already has "env": "prod"
	// This triggers auto-skip and removes from ToSet, resulting in empty
	existingTags := StagedTags{
		ToSet: map[string]string{"env": "prod"},
	}
	action := TagActionTag{
		Tags:           map[string]string{"env": "prod"},
		CurrentAWSTags: map[string]string{"env": "prod"}, // Same value on AWS - auto-skip
	}
	existingValue := "existing-value"
	entryState := EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}}

	result, err := executor.ExecuteTag(t.Context(), staging.ServiceParam, "/app/config", entryState, existingTags, action, nil)
	require.NoError(t, err)

	// Result should be completely empty (no ToSet, no ToUnset)
	assert.Empty(t, result.NewStagedTags.ToSet)
	assert.Empty(t, result.NewStagedTags.ToUnset)
	assert.True(t, result.NewStagedTags.IsEmpty())

	// Should be unstaged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestExecuteTag_UnstageWhenAlreadyNotStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	executor := NewExecutor(store)

	// No pre-staged tags, and action with auto-skip results in empty tags
	existingTags := StagedTags{}
	action := TagActionTag{
		Tags:           map[string]string{"env": "prod"},
		CurrentAWSTags: map[string]string{"env": "prod"}, // Same value on AWS - auto-skip
	}
	existingValue := "existing-value"
	entryState := EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}}

	result, err := executor.ExecuteTag(t.Context(), staging.ServiceParam, "/app/config", entryState, existingTags, action, nil)
	require.NoError(t, err)

	// Result is empty, and unstaging non-existing tags should not error
	assert.True(t, result.NewStagedTags.IsEmpty())
}

func TestLoadEntryState(t *testing.T) {
	t.Parallel()

	t.Run("not staged", func(t *testing.T) {
		t.Parallel()
		store := testutil.NewMockStore()

		currentValue := "aws-value"
		state, err := LoadEntryState(t.Context(), store, staging.ServiceParam, "/app/config", &currentValue)
		require.NoError(t, err)

		assert.Equal(t, &currentValue, state.CurrentValue)
		_, isNotStaged := state.StagedState.(EntryStagedStateNotStaged)
		assert.True(t, isNotStaged)
	})

	t.Run("staged create", func(t *testing.T) {
		t.Parallel()
		store := testutil.NewMockStore()
		require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("draft"),
			StagedAt:  time.Now(),
		}))

		state, err := LoadEntryState(t.Context(), store, staging.ServiceParam, "/app/new", nil)
		require.NoError(t, err)

		create, isCreate := state.StagedState.(EntryStagedStateCreate)
		assert.True(t, isCreate)
		assert.Equal(t, "draft", create.DraftValue)
	})

	t.Run("staged update", func(t *testing.T) {
		t.Parallel()
		store := testutil.NewMockStore()
		require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated"),
			StagedAt:  time.Now(),
		}))

		currentValue := "current"
		state, err := LoadEntryState(t.Context(), store, staging.ServiceParam, "/app/config", &currentValue)
		require.NoError(t, err)

		update, isUpdate := state.StagedState.(EntryStagedStateUpdate)
		assert.True(t, isUpdate)
		assert.Equal(t, "updated", update.DraftValue)
	})

	t.Run("staged delete", func(t *testing.T) {
		t.Parallel()
		store := testutil.NewMockStore()
		require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		}))

		state, err := LoadEntryState(t.Context(), store, staging.ServiceParam, "/app/config", nil)
		require.NoError(t, err)

		_, isDelete := state.StagedState.(EntryStagedStateDelete)
		assert.True(t, isDelete)
	})
}

func TestLoadStagedTags(t *testing.T) {
	t.Parallel()

	t.Run("not staged", func(t *testing.T) {
		t.Parallel()
		store := testutil.NewMockStore()

		tags, baseModifiedAt, err := LoadStagedTags(t.Context(), store, staging.ServiceParam, "/app/config")
		require.NoError(t, err)

		assert.True(t, tags.IsEmpty())
		assert.Nil(t, baseModifiedAt)
	})

	t.Run("staged", func(t *testing.T) {
		t.Parallel()
		store := testutil.NewMockStore()
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:            map[string]string{"env": "prod"},
			Remove:         maputil.NewSet("deprecated"),
			StagedAt:       time.Now(),
			BaseModifiedAt: &baseTime,
		}))

		tags, baseModifiedAt, err := LoadStagedTags(t.Context(), store, staging.ServiceParam, "/app/config")
		require.NoError(t, err)

		assert.Equal(t, "prod", tags.ToSet["env"])
		assert.True(t, tags.ToUnset.Contains("deprecated"))
		assert.Equal(t, baseTime, *baseModifiedAt)
	})
}

// Error path tests using mock store

var errMock = errors.New("mock error")

func TestExecuteEntry_PersistError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.StageEntryErr = errMock
	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: nil,
		StagedState:  EntryStagedStateNotStaged{},
	}

	// Add should fail due to StageEntry error
	_, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/new", state, EntryActionAdd{Value: "new-value"}, nil)
	assert.ErrorIs(t, err, errMock)
}

func TestExecuteEntry_UnstageTagError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.UnstageTagErr = errMock

	// Pre-stage entry so UnstageEntry succeeds
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("draft"),
		StagedAt:  time.Now(),
	}))

	executor := NewExecutor(store)

	state := EntryState{
		CurrentValue: nil,
		StagedState:  EntryStagedStateCreate{DraftValue: "draft"},
	}

	// Delete CREATE with UnstageTag error (not ErrNotStaged)
	_, err := executor.ExecuteEntry(t.Context(), staging.ServiceParam, "/app/new", state, EntryActionDelete{}, nil)
	assert.ErrorIs(t, err, errMock)
}

func TestExecuteTag_PersistError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.StageTagErr = errMock
	executor := NewExecutor(store)

	action := TagActionTag{
		Tags:           map[string]string{"env": "prod"},
		CurrentAWSTags: nil, // nil disables auto-skip
	}
	existingValue := "existing-value"
	entryState := EntryState{CurrentValue: &existingValue, StagedState: EntryStagedStateNotStaged{}}

	// Tag should fail due to StageTag error
	_, err := executor.ExecuteTag(t.Context(), staging.ServiceParam, "/app/config", entryState, StagedTags{}, action, nil)
	assert.ErrorIs(t, err, errMock)
}

func TestLoadEntryState_Error(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetEntryErr = errMock

	_, err := LoadEntryState(t.Context(), store, staging.ServiceParam, "/app/config", nil)
	assert.ErrorIs(t, err, errMock)
}

func TestLoadStagedTags_Error(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetTagErr = errMock

	_, _, err := LoadStagedTags(t.Context(), store, staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, errMock)
}
