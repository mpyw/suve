package update_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/update"
	"github.com/mpyw/suve/internal/model"
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

type mockClient struct {
	getParameterFunc func(ctx context.Context, name string, version string) (*model.Parameter, error)
	putParameterFunc func(ctx context.Context, p *model.Parameter, overwrite bool) (*model.ParameterWriteResult, error)
}

func (m *mockClient) GetParameter(ctx context.Context, name string, version string) (*model.Parameter, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, name, version)
	}

	return nil, errors.New("GetParameter not mocked")
}

func (m *mockClient) PutParameter(ctx context.Context, p *model.Parameter, overwrite bool) (*model.ParameterWriteResult, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, p, overwrite)
	}

	return nil, errors.New("PutParameter not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()

	// Default mock for GetParameter (returns existing parameter)
	defaultGetParameter := func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
		return &model.Parameter{
			Name:  "/app/param",
			Value: "old-value",
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
				putParameterFunc: func(_ context.Context, p *model.Parameter, overwrite bool) (*model.ParameterWriteResult, error) {
					assert.Equal(t, "/app/param", p.Name)
					assert.Equal(t, "test-value", p.Value)

					if meta := p.AWSMeta(); meta != nil {
						assert.Equal(t, "SecureString", meta.Type)
					}

					assert.True(t, overwrite)

					return &model.ParameterWriteResult{
						Name:    "/app/param",
						Version: "2",
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
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
				putParameterFunc: func(_ context.Context, p *model.Parameter, overwrite bool) (*model.ParameterWriteResult, error) {
					assert.Equal(t, "Test description", p.Description)
					assert.True(t, overwrite)

					return &model.ParameterWriteResult{
						Name:    "/app/param",
						Version: "2",
					}, nil
				},
			},
		},
		{
			name:    "update not found error",
			opts:    update.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "parameter not found",
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
					return nil, errors.New("not found")
				},
			},
		},
		{
			name:    "update AWS error",
			opts:    update.Options{Name: "/app/param", Value: "test-value", Type: "String"},
			wantErr: "failed to update parameter",
			mock: &mockClient{
				getParameterFunc: defaultGetParameter,
				putParameterFunc: func(_ context.Context, _ *model.Parameter, _ bool) (*model.ParameterWriteResult, error) {
					return nil, errors.New("AWS error")
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
