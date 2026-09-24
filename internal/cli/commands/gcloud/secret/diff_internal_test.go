// White-box tests for diff.go's argument parsing.
//declscope:namespace diff

package secret

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiffArgs(t *testing.T) {
	t.Parallel()

	t.Run("single spec compares against latest", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#3"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(3)), spec1.Absolute.Version)
		assert.Nil(t, spec2.Absolute.Version)
	})

	t.Run("two specs", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#1", "my-secret#2"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, lo.ToPtr(int64(2)), spec2.Absolute.Version)
	})

	t.Run("mixed format: full spec plus specifier-only", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret#1", "#2"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, lo.ToPtr(int64(2)), spec2.Absolute.Version)
	})

	t.Run("partial spec: name plus specifier-only is swapped", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret", "#3"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(3)), spec1.Absolute.Version)
		assert.Nil(t, spec2.Absolute.Version)
	})

	t.Run("three args: name plus two specifiers", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := parseDiffArgs([]string{"my-secret", "#1", "#2"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, lo.ToPtr(int64(2)), spec2.Absolute.Version)
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

	t.Run("label rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseDiffArgs([]string{"my-secret:latest"})
		require.Error(t, err)
	})
}
