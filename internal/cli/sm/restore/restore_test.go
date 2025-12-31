package restore

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type mockClient struct {
	restoreSecretFunc func(ctx context.Context, params *secretsmanager.RestoreSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error)
}

func (m *mockClient) RestoreSecret(ctx context.Context, params *secretsmanager.RestoreSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error) {
	if m.restoreSecretFunc != nil {
		return m.restoreSecretFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("RestoreSecret not mocked")
}

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "restore secret",
			opts: Options{Name: "my-secret"},
			mock: &mockClient{
				restoreSecretFunc: func(_ context.Context, params *secretsmanager.RestoreSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error) {
					if aws.ToString(params.SecretId) != "my-secret" {
						t.Errorf("expected SecretId my-secret, got %s", aws.ToString(params.SecretId))
					}
					return &secretsmanager.RestoreSecretOutput{
						Name: aws.String("my-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Restored secret")) {
					t.Error("expected 'Restored secret' in output")
				}
				if !bytes.Contains([]byte(output), []byte("my-secret")) {
					t.Error("expected secret name in output")
				}
			},
		},
		{
			name: "error from AWS",
			opts: Options{Name: "my-secret"},
			mock: &mockClient{
				restoreSecretFunc: func(_ context.Context, _ *secretsmanager.RestoreSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.RestoreSecretOutput, error) {
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
