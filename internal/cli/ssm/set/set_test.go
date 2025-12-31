package set_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/cli/ssm/set"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()
	app := &cli.App{
		Name:     "suve",
		Commands: []*cli.Command{set.Command()},
	}

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		err := app.Run([]string{"suve", "set"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()
		err := app.Run([]string{"suve", "set", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("conflicting secure and type flags", func(t *testing.T) {
		t.Parallel()
		err := app.Run([]string{"suve", "set", "--secure", "--type", "String", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})
}

type mockClient struct {
	putParameterFunc func(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

func (m *mockClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("PutParameter not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    set.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "set parameter",
			opts: set.Options{
				Name:  "/app/param",
				Value: "test-value",
				Type:  "SecureString",
			},
			mock: &mockClient{
				putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
					assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
					assert.Equal(t, "test-value", lo.FromPtr(params.Value))
					assert.Equal(t, types.ParameterTypeSecureString, params.Type)
					return &ssm.PutParameterOutput{
						Version: 1,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "/app/param")
				assert.Contains(t, output, "version: 1")
			},
		},
		{
			name: "set with description",
			opts: set.Options{
				Name:        "/app/param",
				Value:       "test-value",
				Type:        "String",
				Description: "Test description",
			},
			mock: &mockClient{
				putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
					assert.Equal(t, "Test description", lo.FromPtr(params.Description))
					return &ssm.PutParameterOutput{
						Version: 1,
					}, nil
				},
			},
		},
		{
			name: "error from AWS",
			opts: set.Options{
				Name:  "/app/param",
				Value: "test-value",
				Type:  "String",
			},
			mock: &mockClient{
				putParameterFunc: func(_ context.Context, _ *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
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
			r := &set.Runner{
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
