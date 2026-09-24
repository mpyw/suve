package param_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmdparam "github.com/mpyw/suve/internal/cli/commands/aws/param"
	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
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

func TestDeleteRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    cmdparam.DeleteOptions
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "delete parameter",
			opts: cmdparam.DeleteOptions{Name: "/app/param"},
			store: &providermock.Store{
				DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Deleted")
				assert.Contains(t, output, "/app/param")
			},
		},
		{
			name: "error from AWS",
			opts: cmdparam.DeleteOptions{Name: "/app/param"},
			store: &providermock.Store{
				DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
					return assert.AnError
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &cmdparam.DeleteRunner{
				UseCase: &param.DeleteUseCase{Store: tt.store},
				Stdout:  &buf,
				Stderr:  &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
