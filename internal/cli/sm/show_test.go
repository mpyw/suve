package sm

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

type mockShowClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockShowClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return m.getSecretValueFunc(ctx, params, optFns...)
}

func (m *mockShowClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	return m.listSecretVersionIdsFunc(ctx, params, optFns...)
}

func TestSMShow(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		spec       *version.Spec
		mock       *mockShowClient
		prettyJSON bool
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			spec: &version.Spec{Name: "my-secret"},
			mock: &mockShowClient{
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
			name: "show with label",
			spec: &version.Spec{Name: "my-secret", Label: aws.String("AWSPREVIOUS")},
			mock: &mockShowClient{
				getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					if aws.ToString(params.VersionStage) != "AWSPREVIOUS" {
						t.Errorf("expected VersionStage AWSPREVIOUS, got %s", aws.ToString(params.VersionStage))
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:          aws.String("my-secret"),
						ARN:           aws.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						VersionId:     aws.String("def456"),
						SecretString:  aws.String("previous-value"),
						VersionStages: []string{"AWSPREVIOUS"},
						CreatedDate:   aws.Time(now.Add(-time.Hour)),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("previous-value")) {
					t.Errorf("output should contain previous value")
				}
			},
		},
		{
			name:       "show with JSON formatting",
			spec:       &version.Spec{Name: "my-secret"},
			prettyJSON: true,
			mock: &mockShowClient{
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						ARN:          aws.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						VersionId:    aws.String("abc123"),
						SecretString: aws.String(`{"key":"value"}`),
						CreatedDate:  &now,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Pretty-printed JSON should have indentation
				if !bytes.Contains([]byte(output), []byte("\"key\"")) {
					t.Errorf("output should contain JSON key")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			err := Show(context.Background(), tt.mock, &buf, tt.spec, tt.prettyJSON)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Show() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Show() unexpected error: %v", err)
				return
			}

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

func TestGetSecretWithVersion(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		spec      *version.Spec
		mock      *mockShowClient
		wantValue string
		wantErr   bool
	}{
		{
			name: "get latest version",
			spec: &version.Spec{Name: "my-secret"},
			mock: &mockShowClient{
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						SecretString: aws.String("current-value"),
					}, nil
				},
			},
			wantValue: "current-value",
		},
		{
			name: "with shift ~1 gets previous version",
			spec: &version.Spec{Name: "my-secret", Shift: 1},
			mock: &mockShowClient{
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
					// Should request v2 (shift 1 from latest)
					if aws.ToString(params.VersionId) != "v2" {
						t.Errorf("expected VersionId v2, got %s", aws.ToString(params.VersionId))
					}
					return &secretsmanager.GetSecretValueOutput{
						Name:         aws.String("my-secret"),
						VersionId:    aws.String("v2"),
						SecretString: aws.String("previous-value"),
					}, nil
				},
			},
			wantValue: "previous-value",
		},
		{
			name: "shift out of range",
			spec: &version.Spec{Name: "my-secret", Shift: 10},
			mock: &mockShowClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: aws.String("v1"), CreatedDate: aws.Time(now.Add(-time.Hour))},
							{VersionId: aws.String("v2"), CreatedDate: &now},
						},
					}, nil
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret, err := GetSecretWithVersion(context.Background(), tt.mock, tt.spec)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetSecretWithVersion() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetSecretWithVersion() unexpected error: %v", err)
				return
			}

			if aws.ToString(secret.SecretString) != tt.wantValue {
				t.Errorf("GetSecretWithVersion() Value = %v, want %v", aws.ToString(secret.SecretString), tt.wantValue)
			}
		})
	}
}
