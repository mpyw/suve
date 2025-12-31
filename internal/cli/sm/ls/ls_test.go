package ls_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/sm/ls"
)

type mockClient struct {
	listSecretsFunc func(ctx context.Context, params *secretsmanager.ListSecretsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
}

func (m *mockClient) ListSecrets(ctx context.Context, params *secretsmanager.ListSecretsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	if m.listSecretsFunc != nil {
		return m.listSecretsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecrets not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    ls.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "list all secrets",
			opts: ls.Options{},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
					return &secretsmanager.ListSecretsOutput{
						SecretList: []types.SecretListEntry{
							{Name: lo.ToPtr("secret1")},
							{Name: lo.ToPtr("secret2")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "secret1")
				assert.Contains(t, output, "secret2")
			},
		},
		{
			name: "list with prefix filter",
			opts: ls.Options{Prefix: "app/"},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, params *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
					require.NotEmpty(t, params.Filters, "expected filter to be set")
					return &secretsmanager.ListSecretsOutput{
						SecretList: []types.SecretListEntry{
							{Name: lo.ToPtr("app/secret1")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "app/secret1")
			},
		},
		{
			name: "error from AWS",
			opts: ls.Options{},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
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
			r := &ls.Runner{
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
