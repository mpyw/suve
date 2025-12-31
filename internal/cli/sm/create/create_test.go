package create

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type mockClient struct {
	createSecretFunc func(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
}

func (m *mockClient) CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	if m.createSecretFunc != nil {
		return m.createSecretFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("CreateSecret not mocked")
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
			name: "create secret",
			opts: Options{Name: "my-secret", Value: "secret-value"},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
					if aws.ToString(params.Name) != "my-secret" {
						t.Errorf("expected name my-secret, got %s", aws.ToString(params.Name))
					}
					if aws.ToString(params.SecretString) != "secret-value" {
						t.Errorf("expected value secret-value, got %s", aws.ToString(params.SecretString))
					}
					return &secretsmanager.CreateSecretOutput{
						Name:      aws.String("my-secret"),
						VersionId: aws.String("abc123"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Created secret")) {
					t.Error("expected 'Created secret' in output")
				}
				if !bytes.Contains([]byte(output), []byte("my-secret")) {
					t.Error("expected secret name in output")
				}
			},
		},
		{
			name: "create with description",
			opts: Options{Name: "my-secret", Value: "secret-value", Description: "Test description"},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
					if aws.ToString(params.Description) != "Test description" {
						t.Errorf("expected description 'Test description', got %s", aws.ToString(params.Description))
					}
					return &secretsmanager.CreateSecretOutput{
						Name:      aws.String("my-secret"),
						VersionId: aws.String("abc123"),
					}, nil
				},
			},
		},
		{
			name: "error from AWS",
			opts: Options{Name: "my-secret", Value: "secret-value"},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, _ *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
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
