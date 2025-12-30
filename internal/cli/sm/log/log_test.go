package log

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

type mockClient struct {
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

func TestRun(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		secretName string
		maxResults int32
		mock       *mockClient
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name:       "show version history",
			secretName: "my-secret",
			maxResults: 10,
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					if aws.ToString(params.SecretId) != "my-secret" {
						t.Errorf("expected SecretId my-secret, got %s", aws.ToString(params.SecretId))
					}
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: aws.String("v1"), CreatedDate: aws.Time(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: aws.String("v2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Version")) {
					t.Error("expected 'Version' in output")
				}
				if !bytes.Contains([]byte(output), []byte("AWSCURRENT")) {
					t.Error("expected AWSCURRENT stage in output")
				}
			},
		},
		{
			name:       "error from AWS",
			secretName: "my-secret",
			maxResults: 10,
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(t.Context(), tt.mock, &buf, tt.secretName, tt.maxResults)

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
