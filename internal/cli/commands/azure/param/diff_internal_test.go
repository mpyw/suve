// White-box tests for diff.go's argument parsing.
//declscope:namespace diff

package param

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiffArgs(t *testing.T) {
	t.Parallel()

	t.Run("single bare key compares against itself", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"key-a"})
		require.NoError(t, err)
		assert.Equal(t, "key-a", spec1.Name)
		assert.Equal(t, "key-a", spec2.Name)
	})

	t.Run("two bare keys compared", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"key-a", "key-b"})
		require.NoError(t, err)
		assert.Equal(t, "key-a", spec1.Name)
		assert.Equal(t, "key-b", spec2.Name)
	})

	t.Run("single key containing hash compares against itself", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-key#1"})
		require.NoError(t, err)
		assert.Equal(t, "my-key#1", spec1.Name)
		assert.Equal(t, "my-key#1", spec2.Name)
	})

	t.Run("two keys where the second contains a hash", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-key", "#1"})
		require.NoError(t, err)
		assert.Equal(t, "my-key", spec1.Name)
		assert.Equal(t, "#1", spec2.Name)
	})

	t.Run("three args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseDiffArgs([]string{"my-key", "#1", "#2"})
		require.Error(t, err)
	})

	t.Run("no args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseDiffArgs([]string{})
		require.Error(t, err)
	})

	t.Run("too many args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseDiffArgs([]string{"a", "b", "c", "d"})
		require.Error(t, err)
	})
}
