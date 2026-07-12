package maputil_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/maputil"
)

func TestSortedKeys(t *testing.T) {
	t.Parallel()

	t.Run("string keys", func(t *testing.T) {
		t.Parallel()

		m := map[string]int{
			"c": 3,
			"a": 1,
			"b": 2,
		}
		keys := slices.Collect(maputil.SortedKeys(m))
		assert.Equal(t, []string{"a", "b", "c"}, keys)
	})

	t.Run("int keys", func(t *testing.T) {
		t.Parallel()

		m := map[int]string{
			3: "c",
			1: "a",
			2: "b",
		}
		keys := slices.Collect(maputil.SortedKeys(m))
		assert.Equal(t, []int{1, 2, 3}, keys)
	})

	t.Run("empty map", func(t *testing.T) {
		t.Parallel()

		m := map[string]int{}
		keys := slices.Collect(maputil.SortedKeys(m))
		assert.Empty(t, keys)
	})

	t.Run("single element", func(t *testing.T) {
		t.Parallel()

		m := map[string]int{"only": 1}
		keys := slices.Collect(maputil.SortedKeys(m))
		assert.Equal(t, []string{"only"}, keys)
	})
}

func TestSortedNames(t *testing.T) {
	t.Parallel()

	type item struct {
		Name  string
		Value int
	}

	t.Run("multiple items", func(t *testing.T) {
		t.Parallel()

		items := []item{
			{Name: "charlie", Value: 3},
			{Name: "alice", Value: 1},
			{Name: "bob", Value: 2},
		}
		names := slices.Collect(maputil.SortedNames(items, func(i item) string { return i.Name }))
		assert.Equal(t, []string{"alice", "bob", "charlie"}, names)
	})

	t.Run("duplicate names", func(t *testing.T) {
		t.Parallel()

		items := []item{
			{Name: "alice", Value: 1},
			{Name: "alice", Value: 2},
			{Name: "bob", Value: 3},
		}
		names := slices.Collect(maputil.SortedNames(items, func(i item) string { return i.Name }))
		assert.Equal(t, []string{"alice", "bob"}, names)
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()

		items := []item{}
		names := slices.Collect(maputil.SortedNames(items, func(i item) string { return i.Name }))
		assert.Empty(t, names)
	})
}
