package diff

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type mockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		secretName string
		version1   string
		version2   string
		mock       *mockClient
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name:       "diff between two versions",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					stage := aws.ToString(params.VersionStage)
					if stage == "AWSPREVIOUS" {
						return &secretsmanager.GetSecretValueOutput{
							Name:         aws.String("my-secret"),
							VersionId:    aws.String("prev-version-id"),
							SecretString: aws.String("old-secret"),
						}, nil
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("curr-version-id"),
						SecretString: aws.String("new-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("-old-secret")) {
					t.Error("expected '-old-secret' in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new-secret")) {
					t.Error("expected '+new-secret' in diff output")
				}
			},
		},
		{
			name:       "no diff when same content",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("version-id"),
						SecretString: aws.String("same-content"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if bytes.Contains([]byte(output), []byte("-same-content")) {
					t.Error("expected no diff for identical content")
				}
			},
		},
		{
			name:       "short version id",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					stage := aws.ToString(params.VersionStage)
					if stage == "AWSPREVIOUS" {
						return &secretsmanager.GetSecretValueOutput{
							Name:         aws.String("my-secret"),
							VersionId:    aws.String("v1"), // Short version ID
							SecretString: aws.String("old"),
						}, nil
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("v2"), // Short version ID
						SecretString: aws.String("new"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Should not panic with short version IDs
				if !bytes.Contains([]byte(output), []byte("-old")) {
					t.Error("expected '-old' in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new")) {
					t.Error("expected '+new' in diff output")
				}
			},
		},
		{
			name:       "error getting first version",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					if aws.ToString(params.VersionStage) == "AWSPREVIOUS" {
						return nil, fmt.Errorf("version not found")
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("version-id"),
						SecretString: aws.String("secret"),
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name:       "error getting second version",
			secretName: "my-secret",
			version1:   ":AWSPREVIOUS",
			version2:   ":AWSCURRENT",
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					if aws.ToString(params.VersionStage) == "AWSCURRENT" {
						return nil, fmt.Errorf("version not found")
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("version-id"),
						SecretString: aws.String("secret"),
					}, nil
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(context.Background(), tt.mock, &buf, tt.secretName, tt.version1, tt.version2)

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
