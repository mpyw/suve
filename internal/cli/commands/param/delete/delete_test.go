package delete_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/delete"
	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "delete"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve param delete")
	})
}

type mockClient struct {
	deleteParameterFunc func(ctx context.Context, name string) error
	getParameterFunc    func(ctx context.Context, name string, version string) (*model.Parameter, error)
}

func (m *mockClient) DeleteParameter(ctx context.Context, name string) error {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, name)
	}

	return errors.New("DeleteParameter not mocked")
}

func (m *mockClient) GetParameter(ctx context.Context, name string, version string) (*model.Parameter, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, name, version)
	}

	return nil, errors.New("not found")
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    delete.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "delete parameter",
			opts: delete.Options{Name: "/app/param"},
			mock: &mockClient{
				deleteParameterFunc: func(_ context.Context, _ string) error {
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
			opts: delete.Options{Name: "/app/param"},
			mock: &mockClient{
				deleteParameterFunc: func(_ context.Context, _ string) error {
					return errors.New("AWS error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &delete.Runner{
				UseCase: &param.DeleteUseCase{Client: tt.mock},
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
