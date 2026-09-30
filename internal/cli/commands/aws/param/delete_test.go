package param_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
)

func TestDeleteCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()
		err := app.Run(t.Context(), []string{"suve", "param", "delete"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve aws param delete")
	})
}
