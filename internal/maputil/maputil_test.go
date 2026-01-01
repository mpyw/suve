package maputil_test

import (
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
		keys := maputil.SortedKeys(m)
		assert.Equal(t, []string{"a", "b", "c"}, keys)
	})

	t.Run("int keys", func(t *testing.T) {
		t.Parallel()
		m := map[int]string{
			3: "c",
			1: "a",
			2: "b",
		}
		keys := maputil.SortedKeys(m)
		assert.Equal(t, []int{1, 2, 3}, keys)
	})

	t.Run("empty map", func(t *testing.T) {
		t.Parallel()
		m := map[string]int{}
		keys := maputil.SortedKeys(m)
		assert.Empty(t, keys)
	})

	t.Run("single element", func(t *testing.T) {
		t.Parallel()
		m := map[string]int{"only": 1}
		keys := maputil.SortedKeys(m)
		assert.Equal(t, []string{"only"}, keys)
	})
}
