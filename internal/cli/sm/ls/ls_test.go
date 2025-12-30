package ls

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

type mockClient struct {
	listSecretsFunc func(ctx context.Context, params *secretsmanager.ListSecretsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
}

func (m *mockClient) ListSecrets(ctx context.Context, params *secretsmanager.ListSecretsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	if m.listSecretsFunc != nil {
		return m.listSecretsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecrets not mocked")
}

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name:   "list all secrets",
			prefix: "",
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
					return &secretsmanager.ListSecretsOutput{
						SecretList: []types.SecretListEntry{
							{Name: aws.String("secret1")},
							{Name: aws.String("secret2")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("secret1")) {
					t.Error("expected secret1 in output")
				}
				if !bytes.Contains([]byte(output), []byte("secret2")) {
					t.Error("expected secret2 in output")
				}
			},
		},
		{
			name:   "list with prefix filter",
			prefix: "app/",
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, params *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
					if len(params.Filters) == 0 {
						t.Error("expected filter to be set")
					}
					return &secretsmanager.ListSecretsOutput{
						SecretList: []types.SecretListEntry{
							{Name: aws.String("app/secret1")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("app/secret1")) {
					t.Error("expected app/secret1 in output")
				}
			},
		},
		{
			name: "error from AWS",
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(context.Background(), tt.mock, &buf, tt.prefix)

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
