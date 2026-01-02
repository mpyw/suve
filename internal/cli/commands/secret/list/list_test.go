package list_test

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
	"github.com/mpyw/suve/internal/cli/commands/secret/list"
)

func TestCommand_Help(t *testing.T) {
	t.Parallel()
	app := appcli.MakeApp()
	var buf bytes.Buffer
	app.Writer = &buf
	err := app.Run(context.Background(), []string{"suve", "secret", "list", "--help"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "List secrets")
}

type mockClient struct {
	listSecretsFunc func(ctx context.Context, params *secretapi.ListSecretsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error)
}

func (m *mockClient) ListSecrets(ctx context.Context, params *secretapi.ListSecretsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
	if m.listSecretsFunc != nil {
		return m.listSecretsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecrets not mocked")
}

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    list.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "list all secrets",
			opts: list.Options{},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
					return &secretapi.ListSecretsOutput{
						SecretList: []secretapi.SecretListEntry{
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
			opts: list.Options{Prefix: "app/"},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, params *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
					require.NotEmpty(t, params.Filters, "expected filter to be set")
					return &secretapi.ListSecretsOutput{
						SecretList: []secretapi.SecretListEntry{
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
			opts: list.Options{},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
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
			r := &list.Runner{
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
