// These are stage.go's tests, so they share its core namespace.
//declscope:core

package staging

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
)

func TestState_IsEmpty(t *testing.T) {
	t.Parallel()

	t.Run("nil state is empty", func(t *testing.T) {
		t.Parallel()

		var state *State
		assert.True(t, state.IsEmpty())
	})

	t.Run("empty state is empty", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		assert.True(t, state.IsEmpty())
	})

	t.Run("state with entry is not empty", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][EntryKey{Name: "/app/config"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("value"),
			StagedAt:  time.Now(),
		}
		assert.False(t, state.IsEmpty())
	})

	t.Run("state with tag is not empty", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Tags[ServiceParam][EntryKey{Name: "/app/config"}] = TagEntry{
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

		state1 := NewEmptyState()
		state1.Entries[ServiceParam][EntryKey{Name: "/app/config1"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("value1"),
			StagedAt:  time.Now(),
		}

		state2 := NewEmptyState()
		state2.Entries[ServiceParam][EntryKey{Name: "/app/config2"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("value2"),
			StagedAt:  time.Now(),
		}

		state1.Merge(state2)

		assert.Len(t, state1.Entries[ServiceParam], 2)
		assert.Contains(t, state1.Entries[ServiceParam], EntryKey{Name: "/app/config1"})
		assert.Contains(t, state1.Entries[ServiceParam], EntryKey{Name: "/app/config2"})
	})

	t.Run("merge overwrites existing entries", func(t *testing.T) {
		t.Parallel()

		state1 := NewEmptyState()
		state1.Entries[ServiceParam][EntryKey{Name: "/app/config"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("old-value"),
			StagedAt:  time.Now(),
		}

		state2 := NewEmptyState()
		state2.Entries[ServiceParam][EntryKey{Name: "/app/config"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("new-value"),
			StagedAt:  time.Now(),
		}

		state1.Merge(state2)

		assert.Equal(t, "new-value", lo.FromPtr(state1.Entries[ServiceParam][EntryKey{Name: "/app/config"}].Value))
	})

	t.Run("merge tags", func(t *testing.T) {
		t.Parallel()

		state1 := NewEmptyState()
		state1.Tags[ServiceParam][EntryKey{Name: "/app/config1"}] = TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state2 := NewEmptyState()
		state2.Tags[ServiceParam][EntryKey{Name: "/app/config2"}] = TagEntry{
			Add:      map[string]string{"team": "backend"},
			StagedAt: time.Now(),
		}

		state1.Merge(state2)

		assert.Len(t, state1.Tags[ServiceParam], 2)
	})

	t.Run("merge nil state does nothing", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][EntryKey{Name: "/app/config"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("value"),
			StagedAt:  time.Now(),
		}

		state.Merge(nil)

		assert.Len(t, state.Entries[ServiceParam], 1)
		assert.Equal(t, "value", lo.FromPtr(state.Entries[ServiceParam][EntryKey{Name: "/app/config"}].Value))
	})

	t.Run("merge into nil maps initializes them", func(t *testing.T) {
		t.Parallel()

		state1 := &State{
			Entries: nil,
			Tags:    nil,
		}

		state2 := NewEmptyState()
		state2.Entries[ServiceParam][EntryKey{Name: "/app/config"}] = Entry{
			Operation: OperationCreate,
			Value:     new("new-value"),
			StagedAt:  time.Now(),
		}
		state2.Tags[ServiceSecret][EntryKey{Name: "my-secret"}] = TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state1.Merge(state2)

		assert.NotNil(t, state1.Entries)
		assert.NotNil(t, state1.Tags)
		assert.Equal(t, "new-value", lo.FromPtr(state1.Entries[ServiceParam][EntryKey{Name: "/app/config"}].Value))
		assert.Equal(t, "prod", state1.Tags[ServiceSecret][EntryKey{Name: "my-secret"}].Add["env"])
	})

	t.Run("merge into nil service maps", func(t *testing.T) {
		t.Parallel()

		state1 := &State{
			Entries: make(map[Service]map[EntryKey]Entry),
			Tags:    make(map[Service]map[EntryKey]TagEntry),
		}

		state2 := NewEmptyState()
		state2.Entries[ServiceParam][EntryKey{Name: "/app/config"}] = Entry{
			Operation: OperationCreate,
			Value:     new("value"),
			StagedAt:  time.Now(),
		}

		state1.Merge(state2)

		assert.NotNil(t, state1.Entries[ServiceParam])
		assert.Len(t, state1.Entries[ServiceParam], 1)
	})

	t.Run("merge unions tag deltas for the same key", func(t *testing.T) {
		t.Parallel()

		key := EntryKey{Name: "/app/config"}

		working := NewEmptyState()
		working.Tags[ServiceParam][key] = TagEntry{
			Add:      map[string]string{"env": "prod"},
			Remove:   maputil.NewSet("old"),
			StagedAt: time.Now(),
		}

		envelope := NewEmptyState()
		envelope.Tags[ServiceParam][key] = TagEntry{
			Add:      map[string]string{"team": "foo"},
			StagedAt: time.Now(),
		}

		working.Merge(envelope)

		merged := working.Tags[ServiceParam][key]
		assert.Equal(t, "prod", merged.Add["env"])
		assert.Equal(t, "foo", merged.Add["team"])
		assert.True(t, merged.Remove.Contains("old"))
	})

	t.Run("merge lets envelope win per tag-key and on add-vs-remove clash", func(t *testing.T) {
		t.Parallel()

		key := EntryKey{Name: "/app/config"}

		working := NewEmptyState()
		working.Tags[ServiceParam][key] = TagEntry{
			// env:    same tag-key Add in both -> envelope value wins.
			// keep:   working Add, envelope silent -> preserved.
			// revive: working Remove, envelope Add -> envelope Add wins.
			// gone:   working Remove, envelope silent -> preserved.
			// drop:   working Add, envelope Remove -> envelope Remove wins.
			Add:      map[string]string{"env": "stage", "keep": "yes", "drop": "no"},
			Remove:   maputil.NewSet("gone", "revive"),
			StagedAt: time.Now(),
		}

		envelope := NewEmptyState()
		envelope.Tags[ServiceParam][key] = TagEntry{
			Add:      map[string]string{"env": "prod", "revive": "back"},
			Remove:   maputil.NewSet("drop"),
			StagedAt: time.Now(),
		}

		working.Merge(envelope)

		merged := working.Tags[ServiceParam][key]
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

		var state State

		err := json.Unmarshal([]byte(`{"version":2}`), &state)
		require.ErrorIs(t, err, ErrStateVersionTooOld)
		require.NotErrorIs(t, err, errStateVersionTooNew)
		assert.True(t, state.IsEmpty())
	})

	t.Run("newer version is an error and is not rewritten", func(t *testing.T) {
		t.Parallel()

		var state State

		err := json.Unmarshal([]byte(`{"version":4}`), &state)
		require.Error(t, err)
		assert.ErrorIs(t, err, errStateVersionTooNew)
	})
}

func TestState_UnmarshalJSON_Duplicate(t *testing.T) {
	t.Parallel()

	t.Run("duplicate entry records are rejected", func(t *testing.T) {
		t.Parallel()

		data := `{"version":3,"entries":{"param":[` +
			`{"name":"/app/x","operation":"update","staged_at":"2024-01-01T00:00:00Z"},` +
			`{"name":"/app/x","operation":"delete","staged_at":"2024-01-02T00:00:00Z"}]}}`

		var state State

		err := json.Unmarshal([]byte(data), &state)
		require.Error(t, err)
		assert.ErrorIs(t, err, errDuplicateRecord)
	})

	t.Run("duplicate tag records are rejected", func(t *testing.T) {
		t.Parallel()

		data := `{"version":3,"tags":{"param":[` +
			`{"name":"/app/x","staged_at":"2024-01-01T00:00:00Z"},` +
			`{"name":"/app/x","staged_at":"2024-01-02T00:00:00Z"}]}}`

		var state State

		err := json.Unmarshal([]byte(data), &state)
		require.Error(t, err)
		assert.ErrorIs(t, err, errDuplicateRecord)
	})

	t.Run("same name under distinct namespaces is not a duplicate", func(t *testing.T) {
		t.Parallel()

		data := `{"version":3,"entries":{"param":[` +
			`{"name":"x","namespace":"a","operation":"update","staged_at":"2024-01-01T00:00:00Z"},` +
			`{"name":"x","namespace":"b","operation":"update","staged_at":"2024-01-02T00:00:00Z"}]}}`

		var state State

		err := json.Unmarshal([]byte(data), &state)
		require.NoError(t, err)
		assert.Equal(t, 2, state.EntryCount())
	})
}

func TestState_UnmarshalJSON_ValueType(t *testing.T) {
	t.Parallel()

	key := EntryKey{Name: "/app/x"}

	t.Run("entry written before value_type existed decodes as unset", func(t *testing.T) {
		t.Parallel()

		// A v3 working-store file written before #664 has no "value_type" field.
		// It must load without error and default to the empty (plaintext) type.
		data := `{"version":3,"entries":{"param":[` +
			`{"name":"/app/x","operation":"create","value":"v","staged_at":"2024-01-01T00:00:00Z"}]}}`

		var state State

		require.NoError(t, json.Unmarshal([]byte(data), &state))

		entry := state.Entries[ServiceParam][key]
		assert.Equal(t, OperationCreate, entry.Operation)
		assert.Empty(t, string(entry.ValueType))
	})

	t.Run("entry with value_type decodes it", func(t *testing.T) {
		t.Parallel()

		data := `{"version":3,"entries":{"param":[` +
			`{"name":"/app/x","operation":"create","value":"v","value_type":"secret","staged_at":"2024-01-01T00:00:00Z"}]}}`

		var state State

		require.NoError(t, json.Unmarshal([]byte(data), &state))

		entry := state.Entries[ServiceParam][key]
		assert.Equal(t, domain.ValueTypeSecret, entry.ValueType)
	})

	t.Run("round-trips through marshal", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][key] = Entry{
			Operation: OperationCreate,
			Value:     new("v"),
			ValueType: domain.ValueTypeSecret,
			StagedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		var got State

		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, domain.ValueTypeSecret, got.Entries[ServiceParam][key].ValueType)
	})

	t.Run("unset type is omitted from the marshaled form", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][key] = Entry{
			Operation: OperationCreate,
			Value:     new("v"),
			StagedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "value_type")
	})
}

func TestState_ExtractService(t *testing.T) {
	t.Parallel()

	t.Run("extract from nil state returns empty state", func(t *testing.T) {
		t.Parallel()

		var state *State

		extracted := state.ExtractService(ServiceParam)
		assert.NotNil(t, extracted)
		assert.True(t, extracted.IsEmpty())
	})

	t.Run("extract specific service", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][EntryKey{Name: "/app/param"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("param-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[ServiceSecret][EntryKey{Name: "my-secret"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("secret-value"),
			StagedAt:  time.Now(),
		}

		extracted := state.ExtractService(ServiceParam)

		assert.Len(t, extracted.Entries[ServiceParam], 1)
		assert.Empty(t, extracted.Entries[ServiceSecret])
	})

	t.Run("extract empty service returns copy", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][EntryKey{Name: "/app/param"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("param-value"),
			StagedAt:  time.Now(),
		}

		extracted := state.ExtractService("")

		assert.Len(t, extracted.Entries[ServiceParam], 1)
	})
}

func TestState_RemoveService(t *testing.T) {
	t.Parallel()

	t.Run("remove from nil state does nothing", func(t *testing.T) {
		t.Parallel()

		var state *State
		// Should not panic
		state.RemoveService(ServiceParam)
	})

	t.Run("remove empty service clears all", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][EntryKey{Name: "/app/param"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("param-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[ServiceSecret][EntryKey{Name: "my-secret"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("secret-value"),
			StagedAt:  time.Now(),
		}
		state.Tags[ServiceParam][EntryKey{Name: "/app/param"}] = TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state.RemoveService("")

		assert.Empty(t, state.Entries[ServiceParam])
		assert.Empty(t, state.Entries[ServiceSecret])
		assert.Empty(t, state.Tags[ServiceParam])
		assert.Empty(t, state.Tags[ServiceSecret])
	})

	t.Run("remove specific service", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][EntryKey{Name: "/app/param"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("param-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[ServiceSecret][EntryKey{Name: "my-secret"}] = Entry{
			Operation: OperationUpdate,
			Value:     new("secret-value"),
			StagedAt:  time.Now(),
		}
		state.Tags[ServiceParam][EntryKey{Name: "/app/param"}] = TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		}

		state.RemoveService(ServiceParam)

		assert.Empty(t, state.Entries[ServiceParam])
		assert.Empty(t, state.Tags[ServiceParam])
		assert.Len(t, state.Entries[ServiceSecret], 1)
	})
}

func TestNewEmptyState(t *testing.T) {
	t.Parallel()

	t.Run("creates initialized empty state", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()

		require.NotNil(t, state)
		require.NotNil(t, state.Entries)
		require.NotNil(t, state.Tags)
		assert.NotNil(t, state.Entries[ServiceParam])
		assert.NotNil(t, state.Entries[ServiceSecret])
		assert.NotNil(t, state.Tags[ServiceParam])
		assert.NotNil(t, state.Tags[ServiceSecret])
	})
}

func TestResourceNotFoundError(t *testing.T) {
	t.Parallel()

	t.Run("error message with inner error", func(t *testing.T) {
		t.Parallel()

		err := &ResourceNotFoundError{
			Err: ErrNotStaged,
		}
		assert.Equal(t, ErrNotStaged.Error(), err.Error())
	})

	t.Run("error message without inner error", func(t *testing.T) {
		t.Parallel()

		err := &ResourceNotFoundError{}
		assert.Equal(t, "resource not found", err.Error())
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()

		err := &ResourceNotFoundError{
			Err: ErrNotStaged,
		}
		assert.ErrorIs(t, err, ErrNotStaged)
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

		state := NewEmptyState()
		state.Entries[ServiceParam][EntryKey{Name: "/app/config"}] = Entry{
			Operation: OperationCreate,
			Value:     new("default-ns"),
		}
		state.Entries[ServiceParam][EntryKey{Name: "/app/config", Namespace: "dev"}] = Entry{
			Operation: OperationCreate,
			Value:     new("dev-ns"),
		}

		assert.Len(t, state.Entries[ServiceParam], 2)
		assert.Equal(t, "default-ns",
			lo.FromPtr(state.Entries[ServiceParam][EntryKey{Name: "/app/config"}].Value))
		assert.Equal(t, "dev-ns",
			lo.FromPtr(state.Entries[ServiceParam][EntryKey{Name: "/app/config", Namespace: "dev"}].Value))
	})

	t.Run("v3 on-disk format uses a namespace field and no NUL separator", func(t *testing.T) {
		t.Parallel()

		state := NewEmptyState()
		state.Entries[ServiceParam][EntryKey{Name: "/app/config"}] = Entry{
			Operation: OperationCreate,
			Value:     new("default-ns"),
		}
		state.Entries[ServiceParam][EntryKey{Name: "/app/config", Namespace: "dev"}] = Entry{
			Operation: OperationCreate,
			Value:     new("dev-ns"),
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		encoded := string(data)

		// The named namespace is a structured field, not a NUL-composite key.
		assert.Contains(t, encoded, `"namespace":"dev"`)
		assert.NotContains(t, encoded, "\x00")

		// Round-trips losslessly back into distinct EntryKey-keyed entries.
		var got State
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Len(t, got.Entries[ServiceParam], 2)
		assert.Equal(t, "dev-ns",
			lo.FromPtr(got.Entries[ServiceParam][EntryKey{Name: "/app/config", Namespace: "dev"}].Value))
	})

	t.Run("SortedEntryKeys orders by name then namespace", func(t *testing.T) {
		t.Parallel()

		m := map[EntryKey]Entry{
			{Name: "/b"}:                    {},
			{Name: "/a", Namespace: "dev"}:  {},
			{Name: "/a"}:                    {},
			{Name: "/a", Namespace: "prod"}: {},
		}

		got := SortedEntryKeys(m)
		assert.Equal(t, []EntryKey{
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

		key := EntryKey{Name: "/app/config"}
		assert.Equal(t, "/app/config", key.Label())
	})

	t.Run("non-empty namespace appends a [namespace] badge", func(t *testing.T) {
		t.Parallel()

		key := EntryKey{Name: "/app/config", Namespace: "dev"}
		assert.Equal(t, "/app/config [dev]", key.Label())
	})
}
