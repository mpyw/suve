package param_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
)

// TestCommandValidation checks argument handling that fails before any store is
// resolved (no Azure credentials needed) — and never panics. App Configuration
// keys are unversioned but may legally contain ':' / '#' / '~', so those are
// treated as ordinary key characters rather than rejected as version specifiers.
func TestCommandValidation(t *testing.T) {
	t.Parallel()

	// missingKey / usage errors are raised before store resolution.
	usageTests := []struct {
		name string
		args []string
	}{
		{
			name: "show missing key",
			args: []string{"suve", "azure", "param", "show"},
		},
		{
			name: "create missing args",
			args: []string{"suve", "azure", "param", "create"},
		},
	}

	for _, tt := range usageTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appcli.MakeApp()
			err := app.Run(t.Context(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "usage:")
		})
	}

	// Specifier-like characters are legal key characters: the key is accepted
	// and the command proceeds to store resolution (which fails only because no
	// store is configured), never producing a version-related rejection.
	keyTests := []struct {
		name string
		args []string
	}{
		{"hash in key accepted", []string{"suve", "azure", "param", "show", "my-key#1"}},
		{"tilde in key accepted", []string{"suve", "azure", "param", "show", "my-key~1"}},
		{"colon label-like key accepted", []string{"suve", "azure", "param", "show", "my-key:prod"}},
		{"ASP.NET colon hierarchy accepted", []string{"suve", "azure", "param", "show", "Logging:LogLevel:Default"}},
	}

	for _, tt := range keyTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appcli.MakeApp()
			err := app.Run(t.Context(), tt.args)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "does not support versions")
			assert.Contains(t, err.Error(), "store specified")
		})
	}
}
