// These are load.go's tests.
//declscope:namespace load

package transition

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

const loadTestCurrentValue = "current"

var errLoadMock = errors.New("mock error")

func TestLoadEntryState(t *testing.T) {
	t.Parallel()

	t.Run("not staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		currentValue := "aws-value"
		state, err := LoadEntryState(t.Context(), store, staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, &currentValue)
		require.NoError(t, err)

		assert.Equal(t, &currentValue, state.CurrentValue)
		_, isNotStaged := state.StagedState.(EntryStagedStateNotStaged)
		assert.True(t, isNotStaged)
	})

	t.Run("staged create", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new"}, staging.Entry{
			Operation: staging.OperationCreate,
			Value:     new("draft"),
			StagedAt:  time.Now(),
		}))

		state, err := LoadEntryState(t.Context(), store, staging.ServiceParam, staging.EntryKey{Name: "/app/new"}, nil)
		require.NoError(t, err)

		create, isCreate := state.StagedState.(EntryStagedStateCreate)
		assert.True(t, isCreate)
		assert.Equal(t, "draft", create.DraftValue)
	})

	t.Run("staged update", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("updated"),
			StagedAt:  time.Now(),
		}))

		currentValue := loadTestCurrentValue
		state, err := LoadEntryState(t.Context(), store, staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, &currentValue)
		require.NoError(t, err)

		update, isUpdate := state.StagedState.(EntryStagedStateUpdate)
		assert.True(t, isUpdate)
		assert.Equal(t, "updated", update.DraftValue)
	})

	t.Run("staged delete", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		}))

		state, err := LoadEntryState(t.Context(), store, staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, nil)
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

		tags, baseModifiedAt, err := LoadStagedTags(t.Context(), store, staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)

		assert.True(t, tags.IsEmpty())
		assert.Nil(t, baseModifiedAt)
	})

	t.Run("staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
			Add:            map[string]string{"env": "prod"},
			Remove:         maputil.NewSet("deprecated"),
			StagedAt:       time.Now(),
			BaseModifiedAt: &baseTime,
		}))

		tags, baseModifiedAt, err := LoadStagedTags(t.Context(), store, staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)

		assert.Equal(t, "prod", tags.ToSet["env"])
		assert.True(t, tags.ToUnset.Contains("deprecated"))
		assert.Equal(t, baseTime, *baseModifiedAt)
	})
}

func TestLoadEntryState_Error(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetEntryErr = errLoadMock

	_, err := LoadEntryState(t.Context(), store, staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, nil)
	assert.ErrorIs(t, err, errLoadMock)
}

func TestLoadStagedTags_Error(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetTagErr = errLoadMock

	_, _, err := LoadStagedTags(t.Context(), store, staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	assert.ErrorIs(t, err, errLoadMock)
}
