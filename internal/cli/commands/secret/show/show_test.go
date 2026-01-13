package show_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/show"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "show"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve secret show")
	})

	t.Run("invalid version spec", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "show", "my-secret#"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be followed by")
	})
}

type mockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretapi.ListSecretVersionIDsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error)
	describeSecretFunc       func(ctx context.Context, params *secretapi.DescribeSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error)
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	return m.getSecretValueFunc(ctx, params, optFns...)
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretapi.ListSecretVersionIDsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
	return m.listSecretVersionIdsFunc(ctx, params, optFns...)
}

func (m *mockClient) DescribeSecret(ctx context.Context, params *secretapi.DescribeSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error) {
	if m.describeSecretFunc != nil {
		return m.describeSecretFunc(ctx, params, optFns...)
	}
	return &secretapi.DescribeSecretOutput{}, nil
}

//nolint:funlen // Table-driven test with many cases
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
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
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
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret", Shift: 1}},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
							{VersionId: lo.ToPtr("v2"), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
							{VersionId: lo.ToPtr("v3"), CreatedDate: &now},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
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
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
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
		{
			name: "error from AWS",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "show without optional fields",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						ARN:          lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						SecretString: lo.ToPtr("secret-value"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "my-secret")
				assert.NotContains(t, output, "VersionId")
				assert.NotContains(t, output, "Stages")
				assert.NotContains(t, output, "Created")
			},
		},
		{
			name: "json flag with non-JSON value warns",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						ARN:          lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						SecretString: lo.ToPtr("not json"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "not json")
			},
		},
		{
			name: "raw mode outputs only value",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Raw: true},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						SecretString: lo.ToPtr("raw-secret-value"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Equal(t, "raw-secret-value", output)
			},
		},
		{
			name: "raw mode with shift",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret", Shift: 1}, Raw: true},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
							{VersionId: lo.ToPtr("v2"), CreatedDate: &now},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						SecretString: lo.ToPtr("previous-value"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Equal(t, "previous-value", output)
			},
		},
		{
			name: "raw mode with JSON formatting",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, ParseJSON: true, Raw: true},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						SecretString: lo.ToPtr(`{"zebra":"last","apple":"first"}`),
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
		{
			name: "show with tags",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:          lo.ToPtr("my-secret"),
						ARN:           lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						VersionId:     lo.ToPtr("abc123"),
						SecretString:  lo.ToPtr("secret-value"),
						VersionStages: []string{"AWSCURRENT"},
						CreatedDate:   &now,
					}, nil
				},
				describeSecretFunc: func(_ context.Context, _ *secretapi.DescribeSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error) {
					return &secretapi.DescribeSecretOutput{
						Tags: []secretapi.Tag{
							{Key: lo.ToPtr("Environment"), Value: lo.ToPtr("production")},
							{Key: lo.ToPtr("Team"), Value: lo.ToPtr("backend")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Tags")
				assert.Contains(t, output, "2 tag(s)")
				assert.Contains(t, output, "Environment")
				assert.Contains(t, output, "production")
				assert.Contains(t, output, "Team")
				assert.Contains(t, output, "backend")
			},
		},
		{
			name: "show with tags in JSON output",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Output: output.FormatJSON},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:          lo.ToPtr("my-secret"),
						ARN:           lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						VersionId:     lo.ToPtr("abc123"),
						SecretString:  lo.ToPtr("secret-value"),
						VersionStages: []string{"AWSCURRENT"},
						CreatedDate:   &now,
					}, nil
				},
				describeSecretFunc: func(_ context.Context, _ *secretapi.DescribeSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error) {
					return &secretapi.DescribeSecretOutput{
						Tags: []secretapi.Tag{
							{Key: lo.ToPtr("Environment"), Value: lo.ToPtr("production")},
							{Key: lo.ToPtr("Team"), Value: lo.ToPtr("backend")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, `"tags"`)
				assert.Contains(t, output, `"Environment"`)
				assert.Contains(t, output, `"production"`)
				assert.Contains(t, output, `"Team"`)
				assert.Contains(t, output, `"backend"`)
			},
		},
		{
			name: "JSON output with empty tags shows empty object",
			opts: show.Options{Spec: &secretversion.Spec{Name: "my-secret"}, Output: output.FormatJSON},
			mock: &mockClient{
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:         lo.ToPtr("my-secret"),
						ARN:          lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"),
						SecretString: lo.ToPtr("secret-value"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, `"tags": {}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &show.Runner{
				UseCase: &secret.ShowUseCase{Client: tt.mock},
				Stdout:  &buf,
				Stderr:  &errBuf,
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
