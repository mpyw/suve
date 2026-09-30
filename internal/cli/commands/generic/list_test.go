package generic_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
)

func TestCommand_Help(t *testing.T) {
	t.Parallel()

	t.Run("param", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()

		var buf bytes.Buffer

		app.Writer = &buf
		err := app.Run(t.Context(), []string{"suve", "param", "list", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "List parameters")
		assert.Contains(t, buf.String(), "--recursive")
		assert.Contains(t, buf.String(), "--filter")
		assert.Contains(t, buf.String(), "--show")
	})

	t.Run("secret", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()

		var buf bytes.Buffer

		app.Writer = &buf
		err := app.Run(t.Context(), []string{"suve", "secret", "list", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "List secrets")
		assert.Contains(t, buf.String(), "--filter")
		assert.Contains(t, buf.String(), "--show")
	})
}
