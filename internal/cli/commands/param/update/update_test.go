package update_test

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
	"github.com/mpyw/suve/internal/cli/commands/param/update"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("conflicting secure and type flags", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "update", "--secure", "--type", "String", "/app/param", "value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})
}

//nolint:lll // mock struct fields match AWS SDK interface signatures
type mockClient struct {
	getParameterFunc func(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error)
	putParameterFunc func(ctx context.Context, params *paramapi.PutParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error)
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockClient) GetParameter(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("GetParameter not mocked")
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockClient) PutParameter(ctx context.Context, params *paramapi.PutParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("PutParameter not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()

	// Default mock for GetParameter (returns existing parameter)
	defaultGetParameter := func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
		return &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{
				Name:  lo.ToPtr("/app/param"),
				Value: lo.ToPtr("old-value"),
			},
		}, nil
	}

	tests := []struct {
		name    string
		opts    update.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "update parameter",
			opts: update.Options{
				Name:  "/app/param",
				Value: "test-value",
				Type:  "SecureString",
			},
			mock: &mockClient{
				getParameterFunc: defaultGetParameter,
				//nolint:lll // inline mock function in test table
				putParameterFunc: func(_ context.Context, params *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
					assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
					assert.Equal(t, "test-value", lo.FromPtr(params.Value))
					assert.Equal(t, paramapi.ParameterTypeSecureString, params.Type)
					assert.True(t, lo.FromPtr(params.Overwrite))

					return &paramapi.PutParameterOutput{
						Version: 2,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Updated parameter")
				assert.Contains(t, output, "/app/param")
				assert.Contains(t, output, "version: 2")
			},
		},
		{
			name: "update with description",
			opts: update.Options{
				Name:        "/app/param",
				Value:       "test-value",
				Type:        "String",
				Description: "Test description",
			},
			mock: &mockClient{
				getParameterFunc: defaultGetParameter,
				//nolint:lll // inline mock function in test table
				putParameterFunc: func(_ context.Context, params *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
					assert.Equal(t, "Test description", lo.FromPtr(params.Description))
					assert.True(t, lo.FromPtr(params.Overwrite))

					return &paramapi.PutParameterOutput{
						Version: 2,
					}, nil
				},
			},
		},
		{
			name:    "update not found error",
			opts:    update.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "parameter not found",
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
					return nil, &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")}
				},
			},
		},
		{
			name:    "update AWS error",
			opts:    update.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "failed to update parameter",
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

			r := &update.Runner{
				UseCase: &param.UpdateUseCase{Client: tt.mock},
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
