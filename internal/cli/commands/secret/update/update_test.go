package update_test

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
	"github.com/mpyw/suve/internal/cli/commands/secret/update"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "update"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "update", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

type mockClient struct {
	//nolint:lll // mock function signature
	getSecretValueFunc func(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error)
	//nolint:lll // mock function signature
	putSecretValueFunc func(ctx context.Context, params *secretapi.PutSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error)
	//nolint:lll // mock function signature
	updateSecretFunc   func(ctx context.Context, params *secretapi.UpdateSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error)
}

//nolint:lll // mock function signature
func (m *mockClient) GetSecretValue(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}

	return &secretapi.GetSecretValueOutput{
		ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
	}, nil
}

//nolint:lll // mock function signature
func (m *mockClient) PutSecretValue(ctx context.Context, params *secretapi.PutSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
	if m.putSecretValueFunc != nil {
		return m.putSecretValueFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("PutSecretValue not mocked")
}

//nolint:lll // mock function signature
func (m *mockClient) UpdateSecret(ctx context.Context, params *secretapi.UpdateSecretInput, optFns ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error) {
	if m.updateSecretFunc != nil {
		return m.updateSecretFunc(ctx, params, optFns...)
	}

	return &secretapi.UpdateSecretOutput{}, nil
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
				//nolint:lll // mock function signature
				putSecretValueFunc: func(_ context.Context, params *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Equal(t, "new-value", lo.FromPtr(params.SecretString))

					return &secretapi.PutSecretValueOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("new-version-id"),
						ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Updated secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "update secret with description",
			opts: update.Options{Name: "my-secret", Value: "new-value", Description: "updated description"},
			mock: &mockClient{
				putSecretValueFunc: func(_ context.Context, params *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
					return &secretapi.PutSecretValueOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("new-version-id"),
						ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
					}, nil
				},
				//nolint:lll // mock function signature
				updateSecretFunc: func(_ context.Context, params *secretapi.UpdateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
					assert.Equal(t, "updated description", lo.FromPtr(params.Description))

					return &secretapi.UpdateSecretOutput{
						ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Updated secret")
			},
		},
		{
			name:    "put secret value error",
			opts:    update.Options{Name: "my-secret", Value: "new-value"},
			wantErr: "failed to update secret value",
			mock: &mockClient{
				//nolint:lll // mock function signature
				putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
		},
		{
			name:    "update description error",
			opts:    update.Options{Name: "my-secret", Value: "new-value", Description: "desc"},
			wantErr: "failed to update secret description",
			mock: &mockClient{
				//nolint:lll // mock function signature
				putSecretValueFunc: func(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
					return &secretapi.PutSecretValueOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("new-version-id"),
						ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
					}, nil
				},
				updateSecretFunc: func(_ context.Context, _ *secretapi.UpdateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error) {
					return nil, fmt.Errorf("description update failed")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &update.Runner{
				UseCase: &secret.UpdateUseCase{Client: tt.mock},
				Stdout:  &buf,
				Stderr:  &errBuf,
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
