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
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/cli/sm/create"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()
	app := &cli.App{
		Name:     "suve",
		Commands: []*cli.Command{create.Command()},
	}

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		err := app.Run([]string{"suve", "create"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing value argument", func(t *testing.T) {
		t.Parallel()
		err := app.Run([]string{"suve", "create", "my-secret"})
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
		wantErr bool
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
			name: "error from AWS",
			opts: create.Options{Name: "my-secret", Value: "secret-value"},
			mock: &mockClient{
				createSecretFunc: func(_ context.Context, _ *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
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
