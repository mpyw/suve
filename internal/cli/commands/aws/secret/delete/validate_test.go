package delete_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/aws/secret/delete"
)

func TestValidateDeleteFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		force          bool
		recoveryWindow int
		wantErr        string
	}{
		{
			name:           "no flags",
			force:          false,
			recoveryWindow: 0,
		},
		{
			name:           "window in range lower bound",
			force:          false,
			recoveryWindow: 7,
		},
		{
			name:           "window in range upper bound",
			force:          false,
			recoveryWindow: 30,
		},
		{
			name:           "force alone",
			force:          true,
			recoveryWindow: 0,
		},
		{
			name:           "window below range",
			force:          false,
			recoveryWindow: 6,
			wantErr:        "--recovery-window must be between 7 and 30 days",
		},
		{
			name:           "window above range",
			force:          false,
			recoveryWindow: 31,
			wantErr:        "--recovery-window must be between 7 and 30 days",
		},
		{
			name:           "force combined with window",
			force:          true,
			recoveryWindow: 7,
			wantErr:        "--force and --recovery-window cannot be combined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := delete.ValidateDeleteFlags(tt.force, tt.recoveryWindow)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}
