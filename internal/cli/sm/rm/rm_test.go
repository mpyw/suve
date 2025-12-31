package rm_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	"github.com/mpyw/suve/internal/cli/sm/rm"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "rm"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

type mockClient struct {
	deleteSecretFunc func(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
}

func (m *mockClient) DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	if m.deleteSecretFunc != nil {
		return m.deleteSecretFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("DeleteSecret not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	now := time.Now()
	deletionDate := now.Add(30 * 24 * time.Hour)

	tests := []struct {
		name    string
		opts    rm.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "delete with recovery window",
			opts: rm.Options{Name: "my-secret", Force: false, RecoveryWindow: 30},
			mock: &mockClient{
				deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
					assert.False(t, lo.FromPtr(params.ForceDeleteWithoutRecovery), "expected ForceDeleteWithoutRecovery to be false")
					assert.Equal(t, int64(30), lo.FromPtr(params.RecoveryWindowInDays))
					return &secretsmanager.DeleteSecretOutput{
						Name:         lo.ToPtr("my-secret"),
						DeletionDate: &deletionDate,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Scheduled deletion")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "force delete",
			opts: rm.Options{Name: "my-secret", Force: true},
			mock: &mockClient{
				deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
					assert.True(t, lo.FromPtr(params.ForceDeleteWithoutRecovery), "expected ForceDeleteWithoutRecovery to be true")
					return &secretsmanager.DeleteSecretOutput{
						Name: lo.ToPtr("my-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Permanently deleted")
			},
		},
		{
			name: "error from AWS",
			opts: rm.Options{Name: "my-secret"},
			mock: &mockClient{
				deleteSecretFunc: func(_ context.Context, _ *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
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
