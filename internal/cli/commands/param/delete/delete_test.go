package delete_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/delete"
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
	deleteParameterFunc func(ctx context.Context, params *paramapi.DeleteParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error)
	getParameterFunc    func(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error)
}

func (m *mockClient) DeleteParameter(ctx context.Context, params *paramapi.DeleteParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error) {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("DeleteParameter not mocked")
}

func (m *mockClient) GetParameter(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, &paramapi.ParameterNotFound{}
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
				deleteParameterFunc: func(_ context.Context, _ *paramapi.DeleteParameterInput, _ ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error) {
					return &paramapi.DeleteParameterOutput{}, nil
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
				deleteParameterFunc: func(_ context.Context, _ *paramapi.DeleteParameterInput, _ ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error) {
					return nil, fmt.Errorf("AWS error")
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
