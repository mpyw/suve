package untag_test

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
	"github.com/mpyw/suve/internal/cli/commands/secret/untag"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "secret", "untag"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing key argument", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "secret", "untag", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
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
		opts    untag.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "remove single tag",
			opts: untag.Options{
				Name: "my-secret",
				Keys: []string{"env"},
			},
			mock: &mockClient{
				untagResourceFunc: func(_ context.Context, params *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
					assert.Contains(t, lo.FromPtr(params.SecretId), "arn:aws:secretsmanager")
					assert.Equal(t, []string{"env"}, params.TagKeys)
					return &secretapi.UntagResourceOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Untagged")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "remove multiple tags",
			opts: untag.Options{
				Name: "my-secret",
				Keys: []string{"env", "team"},
			},
			mock: &mockClient{
				untagResourceFunc: func(_ context.Context, params *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
					assert.Len(t, params.TagKeys, 2)
					return &secretapi.UntagResourceOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "2 key(s)")
			},
		},
		{
			name: "describe secret error",
			opts: untag.Options{
				Name: "my-secret",
				Keys: []string{"env"},
			},
			mock: &mockClient{
				describeSecretFunc: func(_ context.Context, _ *secretapi.DescribeSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: "failed to describe secret",
		},
		{
			name: "untag resource error",
			opts: untag.Options{
				Name: "my-secret",
				Keys: []string{"env"},
			},
			mock: &mockClient{
				untagResourceFunc: func(_ context.Context, _ *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: "failed to remove tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			r := &untag.Runner{
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
