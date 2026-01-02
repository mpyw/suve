package update_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/sm/update"
	"github.com/mpyw/suve/internal/tagging"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "update"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "update", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

type mockClient struct {
	putSecretValueFunc func(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	updateSecretFunc   func(ctx context.Context, params *secretsmanager.UpdateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error)
	tagResourceFunc    func(ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error)
	untagResourceFunc  func(ctx context.Context, params *secretsmanager.UntagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error)
}

func (m *mockClient) PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	if m.putSecretValueFunc != nil {
		return m.putSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("PutSecretValue not mocked")
}

func (m *mockClient) UpdateSecret(ctx context.Context, params *secretsmanager.UpdateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error) {
	if m.updateSecretFunc != nil {
		return m.updateSecretFunc(ctx, params, optFns...)
	}
	return &secretsmanager.UpdateSecretOutput{}, nil
}

func (m *mockClient) TagResource(ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
	if m.tagResourceFunc != nil {
		return m.tagResourceFunc(ctx, params, optFns...)
	}
	return &secretsmanager.TagResourceOutput{}, nil
}

func (m *mockClient) UntagResource(ctx context.Context, params *secretsmanager.UntagResourceInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UntagResourceOutput, error) {
	if m.untagResourceFunc != nil {
		return m.untagResourceFunc(ctx, params, optFns...)
	}
	return &secretsmanager.UntagResourceOutput{}, nil
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    update.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "update secret",
			opts: update.Options{Name: "my-secret", Value: "new-value"},
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, params *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Equal(t, "new-value", lo.FromPtr(params.SecretString))
					return &secretsmanager.PutSecretValueOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("new-version-id"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Updated secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "update secret with description",
			opts: update.Options{Name: "my-secret", Value: "new-value", Description: "updated description"},
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, params *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
					return &secretsmanager.PutSecretValueOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("new-version-id"),
					}, nil
				},
				updateSecretFunc: func(_ context.Context, params *secretsmanager.UpdateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Equal(t, "updated description", lo.FromPtr(params.Description))
					return &secretsmanager.UpdateSecretOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Updated secret")
			},
		},
		{
			name: "update secret with tags",
			opts: update.Options{
				Name:  "my-secret",
				Value: "new-value",
				TagChange: &tagging.Change{
					Add:    map[string]string{"env": "prod"},
					Remove: []string{},
				},
			},
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
					return &secretsmanager.PutSecretValueOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("new-version-id"),
					}, nil
				},
				tagResourceFunc: func(_ context.Context, params *secretsmanager.TagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					return &secretsmanager.TagResourceOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Updated secret")
			},
		},
		{
			name:    "put secret value error",
			opts:    update.Options{Name: "my-secret", Value: "new-value"},
			wantErr: "failed to update secret",
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
		},
		{
			name:    "update description error",
			opts:    update.Options{Name: "my-secret", Value: "new-value", Description: "desc"},
			wantErr: "failed to update description",
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
					return &secretsmanager.PutSecretValueOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("new-version-id"),
					}, nil
				},
				updateSecretFunc: func(_ context.Context, _ *secretsmanager.UpdateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error) {
					return nil, fmt.Errorf("description update failed")
				},
			},
		},
		{
			name: "tag application error",
			opts: update.Options{
				Name:  "my-secret",
				Value: "new-value",
				TagChange: &tagging.Change{
					Add:    map[string]string{"env": "prod"},
					Remove: []string{},
				},
			},
			wantErr: "failed to add tags",
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
					return &secretsmanager.PutSecretValueOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("new-version-id"),
					}, nil
				},
				tagResourceFunc: func(_ context.Context, _ *secretsmanager.TagResourceInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.TagResourceOutput, error) {
					return nil, fmt.Errorf("tag error")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &update.Runner{
				Client: tt.mock,
				Stdout: &buf,
				Stderr: &errBuf,
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
