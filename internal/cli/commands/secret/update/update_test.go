package update_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/update"
	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "update"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "update", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

type mockClient struct {
	getSecretFunc    func(ctx context.Context, name string, versionID string, versionStage string) (*model.Secret, error)
	updateSecretFunc func(ctx context.Context, name string, value string) (*model.SecretWriteResult, error)
}

func (m *mockClient) GetSecret(ctx context.Context, name string, versionID string, versionStage string) (*model.Secret, error) {
	if m.getSecretFunc != nil {
		return m.getSecretFunc(ctx, name, versionID, versionStage)
	}

	return &model.Secret{
		Name:  name,
		Value: "old-value",
	}, nil
}

func (m *mockClient) UpdateSecret(ctx context.Context, name string, value string) (*model.SecretWriteResult, error) {
	if m.updateSecretFunc != nil {
		return m.updateSecretFunc(ctx, name, value)
	}

	return nil, errors.New("UpdateSecret not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    update.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "update secret",
			opts: update.Options{Name: "my-secret", Value: "new-value"},
			mock: &mockClient{
				updateSecretFunc: func(_ context.Context, name string, value string) (*model.SecretWriteResult, error) {
					assert.Equal(t, "my-secret", name)
					assert.Equal(t, "new-value", value)

					return &model.SecretWriteResult{
						Name:    "my-secret",
						Version: "new-version-id",
						ARN:     "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Updated secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name:    "update secret error",
			opts:    update.Options{Name: "my-secret", Value: "new-value"},
			wantErr: "failed to update secret",
			mock: &mockClient{
				updateSecretFunc: func(_ context.Context, _ string, _ string) (*model.SecretWriteResult, error) {
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
				UseCase: &secret.UpdateUseCase{Client: tt.mock},
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
