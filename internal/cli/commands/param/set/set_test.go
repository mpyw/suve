package set_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/set"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "param", "set"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "param", "set", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("conflicting secure and type flags", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "param", "set", "--secure", "--type", "String", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})
}

type mockClient struct {
	getParameterFunc func(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error)
	putParameterFunc func(ctx context.Context, params *paramapi.PutParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error)
}

func (m *mockClient) GetParameter(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockClient) PutParameter(ctx context.Context, params *paramapi.PutParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("PutParameter not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()

	// Default mock for GetParameter (returns not found)
	notFoundErr := &paramapi.ParameterNotFound{}
	defaultGetParameter := func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
		return nil, notFoundErr
	}

	tests := []struct {
		name    string
		opts    set.Options
		mock    *mockClient
		wantErr string
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
				getParameterFunc: defaultGetParameter,
				putParameterFunc: func(_ context.Context, params *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
					assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
					assert.Equal(t, "test-value", lo.FromPtr(params.Value))
					assert.Equal(t, paramapi.ParameterTypeSecureString, params.Type)
					return &paramapi.PutParameterOutput{
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
				getParameterFunc: defaultGetParameter,
				putParameterFunc: func(_ context.Context, params *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
					assert.Equal(t, "Test description", lo.FromPtr(params.Description))
					return &paramapi.PutParameterOutput{
						Version: 1,
					}, nil
				},
			},
		},
		{
			name:    "put parameter error",
			opts:    set.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "failed to put parameter",
			mock: &mockClient{
				getParameterFunc: defaultGetParameter,
				putParameterFunc: func(_ context.Context, _ *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &set.Runner{
				UseCase: &param.SetUseCase{Client: tt.mock},
				Stdout:  &buf,
				Stderr:  &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
