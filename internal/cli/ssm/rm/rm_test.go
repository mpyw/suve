package rm_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/cli/ssm/rm"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()
	app := &cli.App{
		Name:     "suve",
		Commands: []*cli.Command{rm.Command()},
	}

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()
		err := app.Run([]string{"suve", "rm"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parameter name required")
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
		opts    rm.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "delete parameter",
			opts: rm.Options{Name: "/app/param"},
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
			opts: rm.Options{Name: "/app/param"},
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
			r := &rm.Runner{
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
