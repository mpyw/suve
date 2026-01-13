package tag_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/tag"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "tag"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing tag argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "tag", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("invalid tag format", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "tag", "my-secret", "invalid"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value")
	})

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "tag", "my-secret", "=value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")
	})
}

type mockClient struct {
	describeSecretFunc func(ctx context.Context, params *secretapi.DescribeSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error)
	tagResourceFunc    func(ctx context.Context, params *secretapi.TagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error)
	untagResourceFunc  func(ctx context.Context, params *secretapi.UntagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error)
}

func (m *mockClient) DescribeSecret(ctx context.Context, params *secretapi.DescribeSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error) {
	if m.describeSecretFunc != nil {
		return m.describeSecretFunc(ctx, params, optFns...)
	}

	return &secretapi.DescribeSecretOutput{
		ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
	}, nil
}

func (m *mockClient) TagResource(ctx context.Context, params *secretapi.TagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
	if m.tagResourceFunc != nil {
		return m.tagResourceFunc(ctx, params, optFns...)
	}

	return &secretapi.TagResourceOutput{}, nil
}

func (m *mockClient) UntagResource(ctx context.Context, params *secretapi.UntagResourceInput, optFns ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
	if m.untagResourceFunc != nil {
		return m.untagResourceFunc(ctx, params, optFns...)
	}

	return &secretapi.UntagResourceOutput{}, nil
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    tag.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "add single tag",
			opts: tag.Options{
				Name: "my-secret",
				Tags: map[string]string{"env": "prod"},
			},
			mock: &mockClient{
				tagResourceFunc: func(_ context.Context, params *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
					assert.Contains(t, lo.FromPtr(params.SecretId), "arn:aws:secretsmanager")
					assert.Len(t, params.Tags, 1)

					return &secretapi.TagResourceOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Tagged")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "add multiple tags",
			opts: tag.Options{
				Name: "my-secret",
				Tags: map[string]string{"env": "prod", "team": "backend"},
			},
			mock: &mockClient{
				tagResourceFunc: func(_ context.Context, params *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
					assert.Len(t, params.Tags, 2)

					return &secretapi.TagResourceOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "2 tag(s)")
			},
		},
		{
			name: "describe secret error",
			opts: tag.Options{
				Name: "my-secret",
				Tags: map[string]string{"env": "prod"},
			},
			mock: &mockClient{
				describeSecretFunc: func(_ context.Context, _ *secretapi.DescribeSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: "failed to describe secret",
		},
		{
			name: "tag resource error",
			opts: tag.Options{
				Name: "my-secret",
				Tags: map[string]string{"env": "prod"},
			},
			mock: &mockClient{
				tagResourceFunc: func(_ context.Context, _ *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: "failed to add tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			r := &tag.Runner{
				UseCase: &secret.TagUseCase{Client: tt.mock},
				Stdout:  &buf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
