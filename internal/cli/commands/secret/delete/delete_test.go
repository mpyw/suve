package delete_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/delete"
	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "delete"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

type mockClient struct {
	getSecretFunc    func(ctx context.Context, name string, versionID string, versionStage string) (*model.Secret, error)
	deleteSecretFunc func(ctx context.Context, name string, forceDelete bool) (*model.SecretDeleteResult, error)
}

func (m *mockClient) GetSecret(ctx context.Context, name string, versionID string, versionStage string) (*model.Secret, error) {
	if m.getSecretFunc != nil {
		return m.getSecretFunc(ctx, name, versionID, versionStage)
	}

	return nil, errors.New("not found")
}

func (m *mockClient) DeleteSecret(ctx context.Context, name string, forceDelete bool) (*model.SecretDeleteResult, error) {
	if m.deleteSecretFunc != nil {
		return m.deleteSecretFunc(ctx, name, forceDelete)
	}

	return nil, errors.New("DeleteSecret not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()

	now := time.Now()
	deletionDate := now.Add(30 * 24 * time.Hour)

	tests := []struct {
		name    string
		opts    delete.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "delete with recovery window",
			opts: delete.Options{Name: "my-secret", Force: false, RecoveryWindow: 30},
			mock: &mockClient{
				deleteSecretFunc: func(_ context.Context, name string, forceDelete bool) (*model.SecretDeleteResult, error) {
					assert.Equal(t, "my-secret", name)
					assert.False(t, forceDelete)

					return &model.SecretDeleteResult{
						Name:         "my-secret",
						DeletionDate: &deletionDate,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Scheduled deletion")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "force delete",
			opts: delete.Options{Name: "my-secret", Force: true},
			mock: &mockClient{
				deleteSecretFunc: func(_ context.Context, name string, forceDelete bool) (*model.SecretDeleteResult, error) {
					assert.Equal(t, "my-secret", name)
					assert.True(t, forceDelete)

					return &model.SecretDeleteResult{
						Name: "my-secret",
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Permanently deleted")
			},
		},
		{
			name: "error from AWS",
			opts: delete.Options{Name: "my-secret"},
			mock: &mockClient{
				deleteSecretFunc: func(_ context.Context, _ string, _ bool) (*model.SecretDeleteResult, error) {
					return nil, errors.New("AWS error")
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
				UseCase: &secret.DeleteUseCase{Client: tt.mock},
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
