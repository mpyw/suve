package delete_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/cli/commands/secret/delete"
	"github.com/mpyw/suve/internal/provider"
	awssecret "github.com/mpyw/suve/internal/provider/aws/secret"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
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

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      delete.Options
		deleteErr error
		wantErr   bool
		checkOpts func(t *testing.T, opts []provider.DeleteOption)
		check     func(t *testing.T, output string)
	}{
		{
			name: "delete with recovery window",
			opts: delete.Options{Name: "my-secret", Force: false, RecoveryWindow: 30},
			checkOpts: func(t *testing.T, opts []provider.DeleteOption) {
				t.Helper()
				require.Len(t, opts, 1)
				rw, ok := opts[0].(awssecret.RecoveryWindow)
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
			opts: delete.Options{Name: "my-secret", Force: true},
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
			opts:      delete.Options{Name: "my-secret"},
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

			r := &delete.Runner{
				UseCase: &secret.DeleteUseCase{Store: store},
				Stdout:  &buf,
				Stderr:  &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

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
