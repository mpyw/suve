package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequireStashDropConfirmation covers the #331 guard: in a non-interactive
// context (a non-TTY writer, e.g. a pipe) `stash drop` must require --yes rather
// than dropping the stash unconfirmed.
func TestRequireStashDropConfirmation(t *testing.T) {
	t.Parallel()

	// A bytes.Buffer has no Fd(), so IsTerminalWriter reports false (non-TTY).
	nonTTY := &bytes.Buffer{}

	t.Run("non-TTY without --yes is refused", func(t *testing.T) {
		t.Parallel()

		err := requireStashDropConfirmation(false, nonTTY)
		require.ErrorIs(t, err, errStashDropNeedsYes)
	})

	t.Run("--yes bypasses the prompt even without a TTY", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, requireStashDropConfirmation(true, nonTTY))
	})

	t.Run("error message points to --yes", func(t *testing.T) {
		t.Parallel()

		err := requireStashDropConfirmation(false, nonTTY)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--yes")
	})
}
