package list_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/list"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/model"
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
	listSecretsResult []*model.SecretListItem
	listSecretsErr    error
	getSecretValues   map[string]string
	getSecretErr      map[string]error
}

func (m *mockClient) ListSecrets(_ context.Context) ([]*model.SecretListItem, error) {
	if m.listSecretsErr != nil {
		return nil, m.listSecretsErr
	}

	return m.listSecretsResult, nil
}

func (m *mockClient) GetSecret(_ context.Context, name, _, _ string) (*model.Secret, error) {
	if m.getSecretErr != nil {
		if err, ok := m.getSecretErr[name]; ok {
			return nil, err
		}
	}

	if m.getSecretValues != nil {
		if value, ok := m.getSecretValues[name]; ok {
			return &model.Secret{Name: name, Value: value}, nil
		}
	}

	return nil, errors.New("not found")
}

func (m *mockClient) GetSecretVersions(_ context.Context, _ string) ([]*model.SecretVersion, error) {
	return nil, errors.New("not implemented")
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
				listSecretsResult: []*model.SecretListItem{
					{Name: "secret1"},
					{Name: "secret2"},
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
				listSecretsResult: []*model.SecretListItem{
					{Name: "app/secret1"},
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
				listSecretsErr: errors.New("AWS error"),
			},
			wantErr: true,
		},
		{
			name: "filter by regex",
			opts: list.Options{Filter: "prod"},
			mock: &mockClient{
				listSecretsResult: []*model.SecretListItem{
					{Name: "app/prod/secret1"},
					{Name: "app/dev/secret2"},
					{Name: "app/prod/secret3"},
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
				listSecretsResult: []*model.SecretListItem{},
			},
			wantErr: true,
		},
		{
			name: "show secret values",
			opts: list.Options{Show: true},
			mock: &mockClient{
				listSecretsResult: []*model.SecretListItem{
					{Name: "secret1"},
					{Name: "secret2"},
				},
				getSecretValues: map[string]string{
					"secret1": "value1",
					"secret2": "value2",
				},
			},
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "secret1\tvalue1")
				assert.Contains(t, out, "secret2\tvalue2")
			},
		},
		{
			name: "JSON output without show",
			opts: list.Options{Output: output.FormatJSON},
			mock: &mockClient{
				listSecretsResult: []*model.SecretListItem{
					{Name: "secret1"},
					{Name: "secret2"},
				},
			},
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "secret1"`)
				assert.Contains(t, out, `"name": "secret2"`)
				assert.NotContains(t, out, `"value"`)
			},
		},
		{
			name: "JSON output with show",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			mock: &mockClient{
				listSecretsResult: []*model.SecretListItem{
					{Name: "secret1"},
				},
				getSecretValues: map[string]string{
					"secret1": "secret-value",
				},
			},
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "secret1"`)
				assert.Contains(t, out, `"value": "secret-value"`)
			},
		},
		{
			name: "JSON output with show and error",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			mock: &mockClient{
				listSecretsResult: []*model.SecretListItem{
					{Name: "error-secret"},
				},
				getSecretErr: map[string]error{
					"error-secret": errors.New("access denied"),
				},
			},
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "error-secret"`)
				assert.Contains(t, out, `access denied`)
			},
		},
		{
			name: "text output with error",
			opts: list.Options{Show: true},
			mock: &mockClient{
				listSecretsResult: []*model.SecretListItem{
					{Name: "error-secret"},
				},
				getSecretErr: map[string]error{
					"error-secret": errors.New("fetch error"),
				},
			},
			check: func(t *testing.T, out string) {
				t.Helper()
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
