package file

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/staging"
)

func TestInitializeStateMaps(t *testing.T) {
	t.Parallel()

	t.Run("nil entries", func(t *testing.T) {
		t.Parallel()

		state := &staging.State{
			Entries: nil,
			Tags:    nil,
		}

		initializeStateMaps(state)

		assert.NotNil(t, state.Entries)
		assert.NotNil(t, state.Entries[staging.ServiceParam])
		assert.NotNil(t, state.Entries[staging.ServiceSecret])
		assert.NotNil(t, state.Tags)
		assert.NotNil(t, state.Tags[staging.ServiceParam])
		assert.NotNil(t, state.Tags[staging.ServiceSecret])
	})

	t.Run("empty entries map", func(t *testing.T) {
		t.Parallel()

		state := &staging.State{
			Entries: make(map[staging.Service]map[string]staging.Entry),
			Tags:    make(map[staging.Service]map[string]staging.TagEntry),
		}

		initializeStateMaps(state)

		assert.NotNil(t, state.Entries[staging.ServiceParam])
		assert.NotNil(t, state.Entries[staging.ServiceSecret])
		assert.NotNil(t, state.Tags[staging.ServiceParam])
		assert.NotNil(t, state.Tags[staging.ServiceSecret])
	})

	t.Run("partial entries map", func(t *testing.T) {
		t.Parallel()

		state := &staging.State{
			Entries: map[staging.Service]map[string]staging.Entry{
				staging.ServiceParam: {"key": staging.Entry{}},
			},
			Tags: map[staging.Service]map[string]staging.TagEntry{
				staging.ServiceSecret: {"key": staging.TagEntry{}},
			},
		}

		initializeStateMaps(state)

		// Should preserve existing data
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Len(t, state.Tags[staging.ServiceSecret], 1)

		// Should initialize missing maps
		assert.NotNil(t, state.Entries[staging.ServiceSecret])
		assert.NotNil(t, state.Tags[staging.ServiceParam])
	})

	t.Run("already initialized", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["key"] = staging.Entry{}
		state.Tags[staging.ServiceSecret]["key"] = staging.TagEntry{}

		initializeStateMaps(state)

		// Should preserve all existing data
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Len(t, state.Tags[staging.ServiceSecret], 1)
	})
}
