package rm

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

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
	now := time.Now()
	deletionDate := now.Add(30 * 24 * time.Hour)

	tests := []struct {
		name    string
		opts    Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "delete with recovery window",
			opts: Options{Name: "my-secret", Force: false, RecoveryWindow: 30},
			mock: &mockClient{
				deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
					if aws.ToBool(params.ForceDeleteWithoutRecovery) {
						t.Error("expected ForceDeleteWithoutRecovery to be false")
					}
					if aws.ToInt64(params.RecoveryWindowInDays) != 30 {
						t.Errorf("expected recovery window 30, got %d", aws.ToInt64(params.RecoveryWindowInDays))
					}
					return &secretsmanager.DeleteSecretOutput{
						Name:         aws.String("my-secret"),
						DeletionDate: &deletionDate,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Scheduled deletion")) {
					t.Error("expected 'Scheduled deletion' in output")
				}
				if !bytes.Contains([]byte(output), []byte("my-secret")) {
					t.Error("expected secret name in output")
				}
			},
		},
		{
			name: "force delete",
			opts: Options{Name: "my-secret", Force: true},
			mock: &mockClient{
				deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
					if !aws.ToBool(params.ForceDeleteWithoutRecovery) {
						t.Error("expected ForceDeleteWithoutRecovery to be true")
					}
					return &secretsmanager.DeleteSecretOutput{
						Name: aws.String("my-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Permanently deleted")) {
					t.Error("expected 'Permanently deleted' in output")
				}
			},
		},
		{
			name: "error from AWS",
			opts: Options{Name: "my-secret"},
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
			var buf, errBuf bytes.Buffer
			r := &Runner{
				Client: tt.mock,
				Stdout: &buf,
				Stderr: &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
