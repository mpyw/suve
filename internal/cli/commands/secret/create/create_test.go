package create_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/create"
	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "create"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "create", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

type mockClient struct {
	createSecretFunc func(ctx context.Context, s *model.Secret) (*model.SecretWriteResult, error)
}

func (m *mockClient) CreateSecret(ctx context.Context, s *model.Secret) (*model.SecretWriteResult, error) {
	if m.createSecretFunc != nil {
		return m.createSecretFunc(ctx, s)
	}

	return nil, errors.New("CreateSecret not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    create.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "create secret",
			opts: create.Options{Name: "my-secret", Value: "secret-value"},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, s *model.Secret) (*model.SecretWriteResult, error) {
					assert.Equal(t, "my-secret", s.Name)
					assert.Equal(t, "secret-value", s.Value)

					return &model.SecretWriteResult{
						Name:    "my-secret",
						Version: "abc123",
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Created secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "create with description",
			opts: create.Options{Name: "my-secret", Value: "secret-value", Description: "Test description"},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, s *model.Secret) (*model.SecretWriteResult, error) {
					assert.Equal(t, "Test description", s.Description)

					return &model.SecretWriteResult{
						Name:    "my-secret",
						Version: "abc123",
					}, nil
				},
			},
		},
		{
			name:    "error from AWS",
			opts:    create.Options{Name: "my-secret", Value: "secret-value"},
			wantErr: "failed to create secret",
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, _ *model.Secret) (*model.SecretWriteResult, error) {
					return nil, errors.New("AWS error")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &create.Runner{
				UseCase: &secret.CreateUseCase{Client: tt.mock},
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
