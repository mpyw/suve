package list_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/list"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Help(t *testing.T) {
	t.Parallel()

	app := appcli.MakeApp()

	var buf bytes.Buffer

	app.Writer = &buf
	err := app.Run(t.Context(), []string{"suve", "secret", "list", "--help"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "List secrets")
	assert.Contains(t, buf.String(), "--filter")
	assert.Contains(t, buf.String(), "--show")
}

type mockClient struct {
	listSecretsFunc    func(ctx context.Context, params *secretapi.ListSecretsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error)
	getSecretValueFunc func(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error)
}

func (m *mockClient) ListSecrets(ctx context.Context, params *secretapi.ListSecretsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
	if m.listSecretsFunc != nil {
		return m.listSecretsFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("ListSecrets not mocked")
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("GetSecretValue not mocked")
}

//nolint:funlen // Table-driven test with many cases
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
				t.Helper()
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
				t.Helper()
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
		{
			name: "filter by regex",
			opts: list.Options{Filter: "prod"},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
					return &secretapi.ListSecretsOutput{
						SecretList: []secretapi.SecretListEntry{
							{Name: lo.ToPtr("app/prod/secret1")},
							{Name: lo.ToPtr("app/dev/secret2")},
							{Name: lo.ToPtr("app/prod/secret3")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "app/prod/secret1")
				assert.NotContains(t, output, "app/dev/secret2")
				assert.Contains(t, output, "app/prod/secret3")
			},
		},
		{
			name: "invalid regex filter",
			opts: list.Options{Filter: "[invalid"},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
					return &secretapi.ListSecretsOutput{}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "show secret values",
			opts: list.Options{Show: true},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
					return &secretapi.ListSecretsOutput{
						SecretList: []secretapi.SecretListEntry{
							{Name: lo.ToPtr("secret1")},
							{Name: lo.ToPtr("secret2")},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					name := lo.FromPtr(params.SecretId)
					values := map[string]string{
						"secret1": "value1",
						"secret2": "value2",
					}

					return &secretapi.GetSecretValueOutput{
						Name:         params.SecretId,
						SecretString: lo.ToPtr(values[name]),
					}, nil
				},
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, "secret1\tvalue1")
				assert.Contains(t, out, "secret2\tvalue2")
			},
		},
		{
			name: "JSON output without show",
			opts: list.Options{Output: output.FormatJSON},
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
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, `"name": "secret1"`)
				assert.Contains(t, out, `"name": "secret2"`)
				assert.NotContains(t, out, `"value"`)
			},
		},
		{
			name: "JSON output with show",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
					return &secretapi.ListSecretsOutput{
						SecretList: []secretapi.SecretListEntry{
							{Name: lo.ToPtr("secret1")},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						Name:         params.SecretId,
						SecretString: lo.ToPtr("secret-value"),
					}, nil
				},
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, `"name": "secret1"`)
				assert.Contains(t, out, `"value": "secret-value"`)
			},
		},
		{
			name: "JSON output with show and error",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
					return &secretapi.ListSecretsOutput{
						SecretList: []secretapi.SecretListEntry{
							{Name: lo.ToPtr("error-secret")},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return nil, errors.New("access denied")
				},
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, `"name": "error-secret"`)
				assert.Contains(t, out, `"error": "access denied"`)
			},
		},
		{
			name: "text output with error",
			opts: list.Options{Show: true},
			mock: &mockClient{
				listSecretsFunc: func(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
					return &secretapi.ListSecretsOutput{
						SecretList: []secretapi.SecretListEntry{
							{Name: lo.ToPtr("error-secret")},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return nil, errors.New("fetch error")
				},
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, "error-secret")
				assert.Contains(t, out, "<error:")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &list.Runner{
				UseCase: &secret.ListUseCase{Client: tt.mock},
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
