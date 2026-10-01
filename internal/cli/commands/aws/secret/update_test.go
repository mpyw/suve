package secret_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
)

func TestUpdateCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "update"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	// The value is now optional (--value-stdin / editor fallback), but a
	// positional value cannot be combined with --value-stdin.
	t.Run("positional value with --value-stdin conflicts", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "update", "my-secret", "value", "--value-stdin"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot combine a positional value with --value-stdin")
	})
}
