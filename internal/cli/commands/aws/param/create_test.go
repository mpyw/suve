package param_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
)

func TestCreateCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	// The value is now optional (--value-stdin / editor fallback), but a
	// positional value cannot be combined with --value-stdin.
	t.Run("positional value with --value-stdin conflicts", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create", "/app/param", "value", "--value-stdin"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot combine a positional value with --value-stdin")
	})

	t.Run("conflicting secure and type flags", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create", "--secure", "--type", "String", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})

	t.Run("invalid tier value", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create", "--tier", "Bogus", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --tier")
	})

	// A typo/wrong-case --type must be rejected, not silently stored as plaintext.
	t.Run("invalid type value", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "create", "--type", "securestring", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --type")
	})
}
