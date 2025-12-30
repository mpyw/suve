package show

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	"github.com/mpyw/suve/internal/version"
)

type mockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return m.getSecretValueFunc(ctx, params, optFns...)
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	return m.listSecretVersionIdsFunc(ctx, params, optFns...)
}

func TestRun(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		spec       *version.Spec
		mock       *mockClient
		prettyJSON bool
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			spec: &version.Spec{Name: "my-secret"},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:          aws.String("my-secret"),
						ARN:           aws.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						VersionId:     aws.String("abc123"),
						SecretString:  aws.String("secret-value"),
						VersionStages: []string{"AWSCURRENT"},
						CreatedDate:   &now,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("my-secret")) {
					t.Errorf("output should contain secret name")
				}
				if !bytes.Contains([]byte(output), []byte("secret-value")) {
					t.Errorf("output should contain secret value")
				}
			},
		},
		{
			name: "show with shift",
			spec: &version.Spec{Name: "my-secret", Shift: 1},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: aws.String("v1"), CreatedDate: aws.Time(now.Add(-2 * time.Hour))},
							{VersionId: aws.String("v2"), CreatedDate: aws.Time(now.Add(-time.Hour))},
							{VersionId: aws.String("v3"), CreatedDate: &now},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("v2"),
						SecretString: aws.String("previous-value"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("previous-value")) {
					t.Errorf("output should contain previous value")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			err := Run(t.Context(), tt.mock, &buf, tt.spec, tt.prettyJSON)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Run() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Run() unexpected error: %v", err)
				return
			}

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
