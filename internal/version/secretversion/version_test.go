package secretversion_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/version/secretversion"
)

func TestTruncateVersionID(t *testing.T) {
	t.Parallel()

	t.Run("long ID - truncate to 8", func(t *testing.T) {
		t.Parallel()

		result := secretversion.TruncateVersionID("abcdefgh-1234-5678-9abc-def012345678")
		assert.Equal(t, "abcdefgh", result)
	})

	t.Run("exactly 8 chars", func(t *testing.T) {
		t.Parallel()

		result := secretversion.TruncateVersionID("12345678")
		assert.Equal(t, "12345678", result)
	})

	t.Run("short ID - no truncation", func(t *testing.T) {
		t.Parallel()

		result := secretversion.TruncateVersionID("abc")
		assert.Equal(t, "abc", result)
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()

		result := secretversion.TruncateVersionID("")
		assert.Empty(t, result)
	})
}
