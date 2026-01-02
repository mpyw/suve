package delete_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/ssm/delete"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "delete"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve ssm delete")
	})
}

type mockClient struct {
	deleteParameterFunc func(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
}

func (m *mockClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("DeleteParameter not mocked")
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
				deleteParameterFunc: func(_ context.Context, _ *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
					return &ssm.DeleteParameterOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Deleted")
				assert.Contains(t, output, "/app/param")
			},
		},
		{
			name: "error from AWS",
			opts: delete.Options{Name: "/app/param"},
			mock: &mockClient{
				deleteParameterFunc: func(_ context.Context, _ *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
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
				Client: tt.mock,
				Stdout: &buf,
				Stderr: &errBuf,
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
