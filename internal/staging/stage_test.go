package staging_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
)

func TestState_IsEmpty(t *testing.T) {
	t.Parallel()

	t.Run("nil state is empty", func(t *testing.T) {
		t.Parallel()

		var state *staging.State
		assert.True(t, state.IsEmpty())
	})

	t.Run("empty state is empty", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		assert.True(t, state.IsEmpty())
	})

	t.Run("state with entry is not empty", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		}
		assert.False(t, state.IsEmpty())
	})

	t.Run("state with tag is not empty", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Tags[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.TagEntry{
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
		state1.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config1"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value1"),
			StagedAt:  time.Now(),
		}

		state2 := staging.NewEmptyState()
		state2.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config2"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value2"),
			StagedAt:  time.Now(),
		}

		state1.Merge(state2)

		assert.Len(t, state1.Entries[staging.ServiceParam], 2)
		assert.Contains(t, state1.Entries[staging.ServiceParam], staging.EntryKey{Name: "/app/config1"})
		assert.Contains(t, state1.Entries[staging.ServiceParam], staging.EntryKey{Name: "/app/config2"})
	})

	t.Run("merge overwrites existing entries", func(t *testing.T) {
		t.Parallel()

		state1 := staging.NewEmptyState()
		state1.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("old-value"),
			StagedAt:  time.Now(),
		}

		state2 := staging.NewEmptyState()
		state2.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		}

		state1.Merge(state2)

		assert.Equal(t, "new-value", lo.FromPtr(state1.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))
	})

	t.Run("merge tags", func(t *testing.T) {
		t.Parallel()

		state1 := staging.NewEmptyState()
		state1.Tags[staging.ServiceParam][staging.EntryKey{Name: "/app/config1"}] = staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state2 := staging.NewEmptyState()
		state2.Tags[staging.ServiceParam][staging.EntryKey{Name: "/app/config2"}] = staging.TagEntry{
			Add:      map[string]string{"team": "backend"},
			StagedAt: time.Now(),
		}

		state1.Merge(state2)

		assert.Len(t, state1.Tags[staging.ServiceParam], 2)
	})

	t.Run("merge nil state does nothing", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		}

		state.Merge(nil)

		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Equal(t, "value", lo.FromPtr(state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))
	})

	t.Run("merge into nil maps initializes them", func(t *testing.T) {
		t.Parallel()

		state1 := &staging.State{
			Entries: nil,
			Tags:    nil,
		}

		state2 := staging.NewEmptyState()
		state2.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		}
		state2.Tags[staging.ServiceSecret][staging.EntryKey{Name: "my-secret"}] = staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state1.Merge(state2)

		assert.NotNil(t, state1.Entries)
		assert.NotNil(t, state1.Tags)
		assert.Equal(t, "new-value", lo.FromPtr(state1.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))
		assert.Equal(t, "prod", state1.Tags[staging.ServiceSecret][staging.EntryKey{Name: "my-secret"}].Add["env"])
	})

	t.Run("merge into nil service maps", func(t *testing.T) {
		t.Parallel()

		state1 := &staging.State{
			Entries: make(map[staging.Service]map[staging.EntryKey]staging.Entry),
			Tags:    make(map[staging.Service]map[staging.EntryKey]staging.TagEntry),
		}

		state2 := staging.NewEmptyState()
		state2.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		}

		state1.Merge(state2)

		assert.NotNil(t, state1.Entries[staging.ServiceParam])
		assert.Len(t, state1.Entries[staging.ServiceParam], 1)
	})

	t.Run("merge unions tag deltas for the same key", func(t *testing.T) {
		t.Parallel()

		key := staging.EntryKey{Name: "/app/config"}

		working := staging.NewEmptyState()
		working.Tags[staging.ServiceParam][key] = staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			Remove:   maputil.NewSet("old"),
			StagedAt: time.Now(),
		}

		envelope := staging.NewEmptyState()
		envelope.Tags[staging.ServiceParam][key] = staging.TagEntry{
			Add:      map[string]string{"team": "foo"},
			StagedAt: time.Now(),
		}

		working.Merge(envelope)

		merged := working.Tags[staging.ServiceParam][key]
		assert.Equal(t, "prod", merged.Add["env"])
		assert.Equal(t, "foo", merged.Add["team"])
		assert.True(t, merged.Remove.Contains("old"))
	})

	t.Run("merge lets envelope win per tag-key and on add-vs-remove clash", func(t *testing.T) {
		t.Parallel()

		key := staging.EntryKey{Name: "/app/config"}

		working := staging.NewEmptyState()
		working.Tags[staging.ServiceParam][key] = staging.TagEntry{
			// env:    same tag-key Add in both -> envelope value wins.
			// keep:   working Add, envelope silent -> preserved.
			// revive: working Remove, envelope Add -> envelope Add wins.
			// gone:   working Remove, envelope silent -> preserved.
			// drop:   working Add, envelope Remove -> envelope Remove wins.
			Add:      map[string]string{"env": "stage", "keep": "yes", "drop": "no"},
			Remove:   maputil.NewSet("gone", "revive"),
			StagedAt: time.Now(),
		}

		envelope := staging.NewEmptyState()
		envelope.Tags[staging.ServiceParam][key] = staging.TagEntry{
			Add:      map[string]string{"env": "prod", "revive": "back"},
			Remove:   maputil.NewSet("drop"),
			StagedAt: time.Now(),
		}

		working.Merge(envelope)

		merged := working.Tags[staging.ServiceParam][key]
		assert.Equal(t, "prod", merged.Add["env"])     // envelope wins same tag-key
		assert.Equal(t, "yes", merged.Add["keep"])     // working-only preserved
		assert.Equal(t, "back", merged.Add["revive"])  // envelope Add beats working Remove
		assert.True(t, merged.Remove.Contains("gone")) // working-only remove preserved
		assert.True(t, merged.Remove.Contains("drop")) // envelope Remove beats working Add
		assert.NotContains(t, merged.Add, "drop")
		assert.NotContains(t, merged.Remove, "revive")
	})
}

func TestState_UnmarshalJSON_Version(t *testing.T) {
	t.Parallel()

	t.Run("older version is dropped as empty with a distinct diagnostic", func(t *testing.T) {
		t.Parallel()

		var state staging.State

		err := json.Unmarshal([]byte(`{"version":2}`), &state)
		require.ErrorIs(t, err, staging.ErrStateVersionTooOld)
		require.NotErrorIs(t, err, staging.ErrStateVersionTooNew)
		assert.True(t, state.IsEmpty())
	})

	t.Run("newer version is an error and is not rewritten", func(t *testing.T) {
		t.Parallel()

		var state staging.State

		err := json.Unmarshal([]byte(`{"version":4}`), &state)
		require.Error(t, err)
		assert.ErrorIs(t, err, staging.ErrStateVersionTooNew)
	})
}

func TestState_UnmarshalJSON_Duplicate(t *testing.T) {
	t.Parallel()

	t.Run("duplicate entry records are rejected", func(t *testing.T) {
		t.Parallel()

		data := `{"version":3,"entries":{"param":[` +
			`{"name":"/app/x","operation":"update","staged_at":"2024-01-01T00:00:00Z"},` +
			`{"name":"/app/x","operation":"delete","staged_at":"2024-01-02T00:00:00Z"}]}}`

		var state staging.State

		err := json.Unmarshal([]byte(data), &state)
		require.Error(t, err)
		assert.ErrorIs(t, err, staging.ErrDuplicateRecord)
	})

	t.Run("duplicate tag records are rejected", func(t *testing.T) {
		t.Parallel()

		data := `{"version":3,"tags":{"param":[` +
			`{"name":"/app/x","staged_at":"2024-01-01T00:00:00Z"},` +
			`{"name":"/app/x","staged_at":"2024-01-02T00:00:00Z"}]}}`

		var state staging.State

		err := json.Unmarshal([]byte(data), &state)
		require.Error(t, err)
		assert.ErrorIs(t, err, staging.ErrDuplicateRecord)
	})

	t.Run("same name under distinct namespaces is not a duplicate", func(t *testing.T) {
		t.Parallel()

		data := `{"version":3,"entries":{"param":[` +
			`{"name":"x","namespace":"a","operation":"update","staged_at":"2024-01-01T00:00:00Z"},` +
			`{"name":"x","namespace":"b","operation":"update","staged_at":"2024-01-02T00:00:00Z"}]}}`

		var state staging.State

		err := json.Unmarshal([]byte(data), &state)
		require.NoError(t, err)
		assert.Equal(t, 2, state.EntryCount())
	})
}

func TestState_ExtractService(t *testing.T) {
	t.Parallel()

	t.Run("extract from nil state returns empty state", func(t *testing.T) {
		t.Parallel()

		var state *staging.State

		extracted := state.ExtractService(staging.ServiceParam)
		assert.NotNil(t, extracted)
		assert.True(t, extracted.IsEmpty())
	})

	t.Run("extract specific service", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/param"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[staging.ServiceSecret][staging.EntryKey{Name: "my-secret"}] = staging.Entry{
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
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/param"}] = staging.Entry{
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

	t.Run("remove from nil state does nothing", func(t *testing.T) {
		t.Parallel()

		var state *staging.State
		// Should not panic
		state.RemoveService(staging.ServiceParam)
	})

	t.Run("remove empty service clears all", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/param"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[staging.ServiceSecret][staging.EntryKey{Name: "my-secret"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		}
		state.Tags[staging.ServiceParam][staging.EntryKey{Name: "/app/param"}] = staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state.RemoveService("")

		assert.Empty(t, state.Entries[staging.ServiceParam])
		assert.Empty(t, state.Entries[staging.ServiceSecret])
		assert.Empty(t, state.Tags[staging.ServiceParam])
		assert.Empty(t, state.Tags[staging.ServiceSecret])
	})

	t.Run("remove specific service", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/param"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[staging.ServiceSecret][staging.EntryKey{Name: "my-secret"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		}
		state.Tags[staging.ServiceParam][staging.EntryKey{Name: "/app/param"}] = staging.TagEntry{
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

// TestEntryKey_NamespaceIdentity replaces the old NUL-composite key tests. It
// pins the new model's invariants: (1) the same name under two namespaces is
// two distinct staged entries; (2) the null/default namespace is the bare name;
// and (3) the v3 on-disk format carries the namespace as a structured field and
// never encodes it with a NUL separator.
func TestEntryKey_NamespaceIdentity(t *testing.T) {
	t.Parallel()

	t.Run("same name under different namespaces are distinct entries", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("default-ns"),
		}
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config", Namespace: "dev"}] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("dev-ns"),
		}

		assert.Len(t, state.Entries[staging.ServiceParam], 2)
		assert.Equal(t, "default-ns",
			lo.FromPtr(state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))
		assert.Equal(t, "dev-ns",
			lo.FromPtr(state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config", Namespace: "dev"}].Value))
	})

	t.Run("v3 on-disk format uses a namespace field and no NUL separator", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("default-ns"),
		}
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config", Namespace: "dev"}] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("dev-ns"),
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		encoded := string(data)

		// The named namespace is a structured field, not a NUL-composite key.
		assert.Contains(t, encoded, `"namespace":"dev"`)
		assert.NotContains(t, encoded, "\x00")

		// Round-trips losslessly back into distinct EntryKey-keyed entries.
		var got staging.State
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Len(t, got.Entries[staging.ServiceParam], 2)
		assert.Equal(t, "dev-ns",
			lo.FromPtr(got.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config", Namespace: "dev"}].Value))
	})

	t.Run("SortedEntryKeys orders by name then namespace", func(t *testing.T) {
		t.Parallel()

		m := map[staging.EntryKey]staging.Entry{
			{Name: "/b"}:                    {},
			{Name: "/a", Namespace: "dev"}:  {},
			{Name: "/a"}:                    {},
			{Name: "/a", Namespace: "prod"}: {},
		}

		got := staging.SortedEntryKeys(m)
		assert.Equal(t, []staging.EntryKey{
			{Name: "/a"},
			{Name: "/a", Namespace: "dev"},
			{Name: "/a", Namespace: "prod"},
			{Name: "/b"},
		}, got)
	})
}

func TestEntryKey_Label(t *testing.T) {
	t.Parallel()

	t.Run("empty namespace renders the bare name", func(t *testing.T) {
		t.Parallel()

		key := staging.EntryKey{Name: "/app/config"}
		assert.Equal(t, "/app/config", key.Label())
	})

	t.Run("non-empty namespace appends a [namespace] badge", func(t *testing.T) {
		t.Parallel()

		key := staging.EntryKey{Name: "/app/config", Namespace: "dev"}
		assert.Equal(t, "/app/config [dev]", key.Label())
	})
}
