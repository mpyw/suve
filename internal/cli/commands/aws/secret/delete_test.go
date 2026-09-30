package secret_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
)

func TestDeleteCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "delete"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("recovery window out of range is rejected before prompt", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "delete", "--recovery-window", "3", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--recovery-window must be between 7 and 30 days")
	})

	t.Run("force combined with recovery window is rejected before prompt", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "delete", "--force", "--recovery-window", "7", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--force and --recovery-window cannot be combined")
	})
}
