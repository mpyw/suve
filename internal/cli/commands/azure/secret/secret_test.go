package secret_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
)

// TestCommandValidation exercises argument/spec validation that fails before any
// provider store is resolved (so no Azure credentials are needed).
func TestCommandValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "create missing args",
			args:    []string{"suve", "azure", "secret", "create"},
			wantErr: "usage:",
		},
		{
			name:    "update missing name",
			args:    []string{"suve", "azure", "secret", "update"},
			wantErr: "usage:",
		},
		{
			// The value is now optional (stdin/editor fallback), but a positional
			// value cannot be combined with --value-stdin.
			name:    "create value with --value-stdin conflicts",
			args:    []string{"suve", "azure", "secret", "create", "my-secret", "value", "--value-stdin"},
			wantErr: "cannot combine a positional value with --value-stdin",
		},
		{
			name:    "delete missing name",
			args:    []string{"suve", "azure", "secret", "delete"},
			wantErr: "usage:",
		},
		{
			name:    "show missing name",
			args:    []string{"suve", "azure", "secret", "show"},
			wantErr: "usage:",
		},
		{
			name:    "show rejects label spec",
			args:    []string{"suve", "azure", "secret", "show", "my-secret:latest"},
			wantErr: "staging labels are not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appcli.MakeApp()
			err := app.Run(t.Context(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
