// White-box tests for delete.go's flag validation.
//declscope:namespace delete

package secret

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/secretsmanager"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
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

			err := validateDeleteFlags(tt.force, tt.recoveryWindow)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestDeleteRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      deleteOptions
		deleteErr error
		wantErr   bool
		checkOpts func(t *testing.T, opts []provider.DeleteOption)
		check     func(t *testing.T, output string)
	}{
		{
			name: "delete with recovery window",
			opts: deleteOptions{name: "my-secret", force: false, recoveryWindow: 30},
			checkOpts: func(t *testing.T, opts []provider.DeleteOption) {
				t.Helper()
				require.Len(t, opts, 1)
				rw, ok := opts[0].(secretsmanager.RecoveryWindow)
				require.True(t, ok, "expected a RecoveryWindow option")
				assert.Equal(t, int64(30), rw.Days)
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Scheduled deletion")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "force delete",
			opts: deleteOptions{name: "my-secret", force: true},
			checkOpts: func(t *testing.T, opts []provider.DeleteOption) {
				t.Helper()
				require.Len(t, opts, 1)
				assert.IsType(t, provider.ForceDelete{}, opts[0])
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Permanently deleted")
			},
		},
		{
			name:      "error from AWS",
			opts:      deleteOptions{name: "my-secret"},
			deleteErr: errors.New("AWS error"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotOpts []provider.DeleteOption

			store := &providermock.Store{
				DeleteFunc: func(_ context.Context, _ string, opts ...provider.DeleteOption) error {
					gotOpts = opts

					return tt.deleteErr
				},
			}

			var buf, errBuf bytes.Buffer

			r := &deleteRunner{
				useCase: &secret.DeleteUseCase{Store: store},
				stdout:  &buf,
				stderr:  &errBuf,
			}
			err := r.run(t.Context(), tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.checkOpts != nil {
				tt.checkOpts(t, gotOpts)
			}

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
