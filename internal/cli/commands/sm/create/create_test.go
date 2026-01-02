package create_test

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
	"github.com/mpyw/suve/internal/cli/commands/sm/create"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "create"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "create", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

type mockClient struct {
	createSecretFunc func(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
}

func (m *mockClient) CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	if m.createSecretFunc != nil {
		return m.createSecretFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("CreateSecret not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    create.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "create secret",
			opts: create.Options{Name: "my-secret", Value: "secret-value"},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.Name))
					assert.Equal(t, "secret-value", lo.FromPtr(params.SecretString))
					return &secretsmanager.CreateSecretOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("abc123"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Created secret")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "create with description",
			opts: create.Options{Name: "my-secret", Value: "secret-value", Description: "Test description"},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
					assert.Equal(t, "Test description", lo.FromPtr(params.Description))
					return &secretsmanager.CreateSecretOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("abc123"),
					}, nil
				},
			},
		},
		{
			name: "create with tags",
			opts: create.Options{
				Name:  "my-secret",
				Value: "secret-value",
				Tags:  map[string]string{"env": "prod", "team": "platform"},
			},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
					assert.Len(t, params.Tags, 2)
					tagMap := make(map[string]string)
					for _, tag := range params.Tags {
						tagMap[lo.FromPtr(tag.Key)] = lo.FromPtr(tag.Value)
					}
					assert.Equal(t, "prod", tagMap["env"])
					assert.Equal(t, "platform", tagMap["team"])
					return &secretsmanager.CreateSecretOutput{
						Name:      lo.ToPtr("my-secret"),
						VersionId: lo.ToPtr("abc123"),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Created secret")
			},
		},
		{
			name:    "error from AWS",
			opts:    create.Options{Name: "my-secret", Value: "secret-value"},
			wantErr: "failed to create secret",
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, _ *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &create.Runner{
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
