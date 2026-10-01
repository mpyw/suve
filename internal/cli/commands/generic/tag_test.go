package generic_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
)

// TestTagCommand_Validation exercises the wired param and secret tag/untag commands
// end-to-end through the app, covering argument validation for both providers.
func TestTagCommand_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"param tag missing arguments", []string{"suve", "param", "tag"}, "usage:"},
		{"param tag missing tag argument", []string{"suve", "param", "tag", "/app/param"}, "usage:"},
		{"param tag invalid tag format", []string{"suve", "param", "tag", "/app/param", "invalid"}, "expected key=value"},
		{"param tag empty key", []string{"suve", "param", "tag", "/app/param", "=value"}, "key cannot be empty"},
		{"param untag missing arguments", []string{"suve", "param", "untag"}, "usage:"},
		{"param untag missing key argument", []string{"suve", "param", "untag", "/app/param"}, "usage:"},
		{"secret tag missing arguments", []string{"suve", "secret", "tag"}, "usage:"},
		{"secret tag missing tag argument", []string{"suve", "secret", "tag", "my-secret"}, "usage:"},
		{"secret tag invalid tag format", []string{"suve", "secret", "tag", "my-secret", "invalid"}, "expected key=value"},
		{"secret tag empty key", []string{"suve", "secret", "tag", "my-secret", "=value"}, "key cannot be empty"},
		{"secret untag missing arguments", []string{"suve", "secret", "untag"}, "usage:"},
		{"secret untag missing key argument", []string{"suve", "secret", "untag", "my-secret"}, "usage:"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := apptest.AWSApp()
			err := app.Run(t.Context(), tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
		})
	}
}
