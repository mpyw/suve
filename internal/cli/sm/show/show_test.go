package show_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/sm/show"
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
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name    string
		opts    show.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show latest version",
			opts: show.Options{Spec: &smversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:          lo.ToPtr("my-secret"),
						ARN:           lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						VersionId:     lo.ToPtr("abc123"),
						SecretString:  lo.ToPtr("secret-value"),
						VersionStages: []string{"AWSCURRENT"},
						CreatedDate:   &now,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "my-secret")
				assert.Contains(t, output, "secret-value")
			},
		},
		{
			name: "show with shift",
			opts: show.Options{Spec: &smversion.Spec{Name: "my-secret", Shift: 1}},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions: []types.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
							{VersionId: lo.ToPtr("v2"), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
							{VersionId: lo.ToPtr("v3"), CreatedDate: &now},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						VersionId:    lo.ToPtr("v2"),
						SecretString: lo.ToPtr("previous-value"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "previous-value")
			},
		},
		{
			name: "show JSON formatted with sorted keys",
			opts: show.Options{Spec: &smversion.Spec{Name: "my-secret"}, JSONFormat: true},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
					return &secretsmanager.GetSecretValueOutput{
						Name:          lo.ToPtr("my-secret"),
						ARN:           lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						VersionId:     lo.ToPtr("abc123"),
						SecretString:  lo.ToPtr(`{"zebra":"last","apple":"first"}`),
						VersionStages: []string{"AWSCURRENT"},
						CreatedDate:   &now,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				appleIdx := bytes.Index([]byte(output), []byte("apple"))
				zebraIdx := bytes.Index([]byte(output), []byte("zebra"))
				require.NotEqual(t, -1, appleIdx, "expected apple in output")
				require.NotEqual(t, -1, zebraIdx, "expected zebra in output")
				assert.Less(t, appleIdx, zebraIdx, "expected keys to be sorted (apple before zebra)")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &show.Runner{
				Client: tt.mock,
				Stdout: &buf,
				Stderr: &errBuf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
