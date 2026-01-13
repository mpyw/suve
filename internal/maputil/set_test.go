package maputil_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/maputil"
)

func TestNewSet(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet[string]()
		assert.Equal(t, 0, s.Len())
	})

	t.Run("with values", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b", "c")
		assert.Equal(t, 3, s.Len())
		assert.True(t, s.Contains("a"))
		assert.True(t, s.Contains("b"))
		assert.True(t, s.Contains("c"))
	})

	t.Run("with duplicates", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b", "a", "c", "b")
		assert.Equal(t, 3, s.Len())
	})

	t.Run("with int type", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet(1, 2, 3)
		assert.Equal(t, 3, s.Len())
		assert.True(t, s.Contains(1))
		assert.True(t, s.Contains(2))
		assert.True(t, s.Contains(3))
	})
}

func TestSet_Add(t *testing.T) {
	t.Parallel()

	t.Run("add new value", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet[string]()
		s.Add("a")
		assert.Equal(t, 1, s.Len())
		assert.True(t, s.Contains("a"))
	})

	t.Run("add existing value", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a")
		s.Add("a")
		assert.Equal(t, 1, s.Len())
	})

	t.Run("add multiple values", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet[string]()
		s.Add("a")
		s.Add("b")
		s.Add("c")
		assert.Equal(t, 3, s.Len())
	})
}

func TestSet_Remove(t *testing.T) {
	t.Parallel()

	t.Run("remove existing value", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b", "c")
		s.Remove("b")
		assert.Equal(t, 2, s.Len())
		assert.True(t, s.Contains("a"))
		assert.False(t, s.Contains("b"))
		assert.True(t, s.Contains("c"))
	})

	t.Run("remove non-existing value", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b")
		s.Remove("c")
		assert.Equal(t, 2, s.Len())
	})

	t.Run("remove from empty set", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet[string]()
		s.Remove("a")
		assert.Equal(t, 0, s.Len())
	})
}

func TestSet_Contains(t *testing.T) {
	t.Parallel()

	t.Run("contains existing value", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b")
		assert.True(t, s.Contains("a"))
		assert.True(t, s.Contains("b"))
	})

	t.Run("does not contain non-existing value", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b")
		assert.False(t, s.Contains("c"))
	})

	t.Run("empty set contains nothing", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet[string]()
		assert.False(t, s.Contains("a"))
	})

	t.Run("nil set contains nothing", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]
		assert.False(t, s.Contains("a"))
	})
}

func TestSet_Len(t *testing.T) {
	t.Parallel()

	t.Run("empty set", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet[string]()
		assert.Equal(t, 0, s.Len())
	})

	t.Run("non-empty set", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b", "c")
		assert.Equal(t, 3, s.Len())
	})

	t.Run("nil set", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]
		assert.Equal(t, 0, s.Len())
	})
}

func TestSet_Values(t *testing.T) {
	t.Parallel()

	t.Run("empty set", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet[string]()
		assert.Empty(t, s.Values())
	})

	t.Run("non-empty set", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b", "c")
		values := s.Values()
		assert.Len(t, values, 3)
		assert.ElementsMatch(t, []string{"a", "b", "c"}, values)
	})

	t.Run("nil set", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]
		assert.Empty(t, s.Values())
	})
}

func TestSet_MarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("empty set", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet[string]()
		data, err := json.Marshal(s)
		require.NoError(t, err)
		assert.Equal(t, "[]", string(data))
	})

	t.Run("single value", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a")
		data, err := json.Marshal(s)
		require.NoError(t, err)
		assert.Equal(t, `["a"]`, string(data))
	})

	t.Run("multiple values", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet("a", "b")
		data, err := json.Marshal(s)
		require.NoError(t, err)
		// Order is not guaranteed, so unmarshal and check
		var values []string
		require.NoError(t, json.Unmarshal(data, &values))
		assert.ElementsMatch(t, []string{"a", "b"}, values)
	})

	t.Run("int set", func(t *testing.T) {
		t.Parallel()

		s := maputil.NewSet(1, 2, 3)
		data, err := json.Marshal(s)
		require.NoError(t, err)

		var values []int
		require.NoError(t, json.Unmarshal(data, &values))
		assert.ElementsMatch(t, []int{1, 2, 3}, values)
	})

	t.Run("nil set", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]

		data, err := json.Marshal(s)
		require.NoError(t, err)
		assert.Equal(t, "[]", string(data))
	})
}

func TestSet_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("empty array", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]

		err := json.Unmarshal([]byte("[]"), &s)
		require.NoError(t, err)
		assert.Equal(t, 0, s.Len())
	})

	t.Run("single value", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]

		err := json.Unmarshal([]byte(`["a"]`), &s)
		require.NoError(t, err)
		assert.Equal(t, 1, s.Len())
		assert.True(t, s.Contains("a"))
	})

	t.Run("multiple values", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]

		err := json.Unmarshal([]byte(`["a", "b", "c"]`), &s)
		require.NoError(t, err)
		assert.Equal(t, 3, s.Len())
		assert.True(t, s.Contains("a"))
		assert.True(t, s.Contains("b"))
		assert.True(t, s.Contains("c"))
	})

	t.Run("duplicates in JSON are deduplicated", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]

		err := json.Unmarshal([]byte(`["a", "b", "a", "c", "b"]`), &s)
		require.NoError(t, err)
		assert.Equal(t, 3, s.Len())
	})

	t.Run("int set", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[int]

		err := json.Unmarshal([]byte(`[1, 2, 3]`), &s)
		require.NoError(t, err)
		assert.Equal(t, 3, s.Len())
		assert.True(t, s.Contains(1))
		assert.True(t, s.Contains(2))
		assert.True(t, s.Contains(3))
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]

		err := json.Unmarshal([]byte(`not json`), &s)
		assert.Error(t, err)
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]

		err := json.Unmarshal([]byte(`{"key": "value"}`), &s)
		assert.Error(t, err)
	})

	t.Run("null", func(t *testing.T) {
		t.Parallel()

		var s maputil.Set[string]

		err := json.Unmarshal([]byte(`null`), &s)
		require.NoError(t, err)
		assert.Equal(t, 0, s.Len())
	})
}

func TestSet_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("string set roundtrip", func(t *testing.T) {
		t.Parallel()

		original := maputil.NewSet("x", "y", "z")
		data, err := json.Marshal(original)
		require.NoError(t, err)

		var restored maputil.Set[string]

		err = json.Unmarshal(data, &restored)
		require.NoError(t, err)

		assert.Equal(t, original.Len(), restored.Len())

		for v := range original {
			assert.True(t, restored.Contains(v))
		}
	})

	t.Run("int set roundtrip", func(t *testing.T) {
		t.Parallel()

		original := maputil.NewSet(10, 20, 30)
		data, err := json.Marshal(original)
		require.NoError(t, err)

		var restored maputil.Set[int]

		err = json.Unmarshal(data, &restored)
		require.NoError(t, err)

		assert.Equal(t, original.Len(), restored.Len())

		for v := range original {
			assert.True(t, restored.Contains(v))
		}
	})
}
