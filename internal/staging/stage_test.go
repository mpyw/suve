package staging_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
)

func TestState_IsEmpty(t *testing.T) {
	t.Parallel()

	t.Run("empty state is empty", func(t *testing.T) {
		t.Parallel()
		state := staging.NewEmptyState()
		assert.True(t, state.IsEmpty())
	})

	t.Run("state with entry is not empty", func(t *testing.T) {
		t.Parallel()
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		}
		assert.False(t, state.IsEmpty())
	})

	t.Run("state with tag is not empty", func(t *testing.T) {
		t.Parallel()
		state := staging.NewEmptyState()
		state.Tags[staging.ServiceParam]["/app/config"] = staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}
		assert.False(t, state.IsEmpty())
	})
}

func TestState_Merge(t *testing.T) {
	t.Parallel()

	t.Run("merge entries", func(t *testing.T) {
		t.Parallel()
		state1 := staging.NewEmptyState()
		state1.Entries[staging.ServiceParam]["/app/config1"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value1"),
			StagedAt:  time.Now(),
		}

		state2 := staging.NewEmptyState()
		state2.Entries[staging.ServiceParam]["/app/config2"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value2"),
			StagedAt:  time.Now(),
		}

		state1.Merge(state2)

		assert.Len(t, state1.Entries[staging.ServiceParam], 2)
		assert.Contains(t, state1.Entries[staging.ServiceParam], "/app/config1")
		assert.Contains(t, state1.Entries[staging.ServiceParam], "/app/config2")
	})

	t.Run("merge overwrites existing entries", func(t *testing.T) {
		t.Parallel()
		state1 := staging.NewEmptyState()
		state1.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("old-value"),
			StagedAt:  time.Now(),
		}

		state2 := staging.NewEmptyState()
		state2.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		}

		state1.Merge(state2)

		assert.Equal(t, "new-value", lo.FromPtr(state1.Entries[staging.ServiceParam]["/app/config"].Value))
	})

	t.Run("merge tags", func(t *testing.T) {
		t.Parallel()
		state1 := staging.NewEmptyState()
		state1.Tags[staging.ServiceParam]["/app/config1"] = staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state2 := staging.NewEmptyState()
		state2.Tags[staging.ServiceParam]["/app/config2"] = staging.TagEntry{
			Add:      map[string]string{"team": "backend"},
			StagedAt: time.Now(),
		}

		state1.Merge(state2)

		assert.Len(t, state1.Tags[staging.ServiceParam], 2)
	})
}

func TestState_ExtractService(t *testing.T) {
	t.Parallel()

	t.Run("extract specific service", func(t *testing.T) {
		t.Parallel()
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/param"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		}

		extracted := state.ExtractService(staging.ServiceParam)

		assert.Len(t, extracted.Entries[staging.ServiceParam], 1)
		assert.Empty(t, extracted.Entries[staging.ServiceSecret])
	})

	t.Run("extract empty service returns copy", func(t *testing.T) {
		t.Parallel()
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/param"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		}

		extracted := state.ExtractService("")

		assert.Len(t, extracted.Entries[staging.ServiceParam], 1)
	})
}

func TestState_RemoveService(t *testing.T) {
	t.Parallel()

	t.Run("remove specific service", func(t *testing.T) {
		t.Parallel()
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/param"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		}
		state.Tags[staging.ServiceParam]["/app/param"] = staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state.RemoveService(staging.ServiceParam)

		assert.Empty(t, state.Entries[staging.ServiceParam])
		assert.Empty(t, state.Tags[staging.ServiceParam])
		assert.Len(t, state.Entries[staging.ServiceSecret], 1)
	})
}

func TestNewEmptyState(t *testing.T) {
	t.Parallel()

	t.Run("creates initialized empty state", func(t *testing.T) {
		t.Parallel()
		state := staging.NewEmptyState()

		require.NotNil(t, state)
		require.NotNil(t, state.Entries)
		require.NotNil(t, state.Tags)
		assert.NotNil(t, state.Entries[staging.ServiceParam])
		assert.NotNil(t, state.Entries[staging.ServiceSecret])
		assert.NotNil(t, state.Tags[staging.ServiceParam])
		assert.NotNil(t, state.Tags[staging.ServiceSecret])
	})
}

func TestResourceNotFoundError(t *testing.T) {
	t.Parallel()

	t.Run("error message with inner error", func(t *testing.T) {
		t.Parallel()
		err := &staging.ResourceNotFoundError{
			Err: staging.ErrNotStaged,
		}
		assert.Equal(t, staging.ErrNotStaged.Error(), err.Error())
	})

	t.Run("error message without inner error", func(t *testing.T) {
		t.Parallel()
		err := &staging.ResourceNotFoundError{}
		assert.Equal(t, "resource not found", err.Error())
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()
		err := &staging.ResourceNotFoundError{
			Err: staging.ErrNotStaged,
		}
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}
