package show

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	"github.com/mpyw/suve/internal/version/smversion"
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
		spec       *smversion.Spec
		mock       *mockClient
		jsonFormat bool
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			spec: &smversion.Spec{Name: "my-secret"},
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
			spec: &smversion.Spec{Name: "my-secret", Shift: 1},
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
		{
			name:       "show JSON formatted with sorted keys",
			spec:       &smversion.Spec{Name: "my-secret"},
			jsonFormat: true,
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:          aws.String("my-secret"),
						ARN:           aws.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						VersionId:     aws.String("abc123"),
						SecretString:  aws.String(`{"zebra":"last","apple":"first"}`),
						VersionStages: []string{"AWSCURRENT"},
						CreatedDate:   &now,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Keys should be sorted (apple before zebra)
				appleIdx := bytes.Index([]byte(output), []byte("apple"))
				zebraIdx := bytes.Index([]byte(output), []byte("zebra"))
				if appleIdx == -1 || zebraIdx == -1 {
					t.Error("expected both keys in output")
					return
				}
				if appleIdx > zebraIdx {
					t.Error("expected keys to be sorted (apple before zebra)")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf, errBuf bytes.Buffer

			err := Run(t.Context(), tt.mock, &buf, &errBuf, tt.spec, tt.jsonFormat)

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
