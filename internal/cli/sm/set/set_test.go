package set

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type mockClient struct {
	putSecretValueFunc func(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
}

func (m *mockClient) PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	if m.putSecretValueFunc != nil {
		return m.putSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("PutSecretValue not mocked")
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		secretName string
		value      string
		mock       *mockClient
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name:       "update secret",
			secretName: "my-secret",
			value:      "new-value",
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, params *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
					if aws.ToString(params.SecretId) != "my-secret" {
						t.Errorf("expected SecretId my-secret, got %s", aws.ToString(params.SecretId))
					}
					if aws.ToString(params.SecretString) != "new-value" {
						t.Errorf("expected value new-value, got %s", aws.ToString(params.SecretString))
					}
					return &secretsmanager.PutSecretValueOutput{
						Name:      aws.String("my-secret"),
						VersionId: aws.String("new-version-id"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Updated secret")) {
					t.Error("expected 'Updated secret' in output")
				}
				if !bytes.Contains([]byte(output), []byte("my-secret")) {
					t.Error("expected secret name in output")
				}
			},
		},
		{
			name:       "error from AWS",
			secretName: "my-secret",
			value:      "new-value",
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(context.Background(), tt.mock, &buf, tt.secretName, tt.value)

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
