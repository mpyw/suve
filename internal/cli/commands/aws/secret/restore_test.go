package secret_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
)

func TestRestoreCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "restore"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}
