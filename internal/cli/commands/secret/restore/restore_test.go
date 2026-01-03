package restore_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/restore"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "secret", "restore"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

type mockClient struct {
	restoreSecretFunc func(ctx context.Context, params *secretapi.RestoreSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.RestoreSecretOutput, error)
}

func (m *mockClient) RestoreSecret(ctx context.Context, params *secretapi.RestoreSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.RestoreSecretOutput, error) {
	if m.restoreSecretFunc != nil {
		return m.restoreSecretFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("RestoreSecret not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    restore.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "restore secret",
			opts: restore.Options{Name: "my-secret"},
			mock: &mockClient{
				restoreSecretFunc: func(_ context.Context, params *secretapi.RestoreSecretInput, _ ...func(*secretapi.Options)) (*secretapi.RestoreSecretOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					return &secretapi.RestoreSecretOutput{
						Name: lo.ToPtr("my-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Restored secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "error from AWS",
			opts: restore.Options{Name: "my-secret"},
			mock: &mockClient{
				restoreSecretFunc: func(_ context.Context, _ *secretapi.RestoreSecretInput, _ ...func(*secretapi.Options)) (*secretapi.RestoreSecretOutput, error) {
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
			r := &restore.Runner{
				UseCase: &secret.RestoreUseCase{Client: tt.mock},
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
