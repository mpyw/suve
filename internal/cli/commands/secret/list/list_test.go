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
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
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

// listStore builds a mock reader returning the given names and per-name
// values/errors (for --show).
func listStore(names []string, values map[string]string, errs map[string]error, listErr error) *providermock.Store {
	return &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return names, listErr
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			if errs != nil {
				if err, ok := errs[name]; ok {
					return nil, err
				}
			}

			return &domain.Entry{Name: name, Value: values[name]}, nil
		},
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    list.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name:  "list all secrets",
			opts:  list.Options{},
			store: listStore([]string{"secret1", "secret2"}, nil, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "secret1")
				assert.Contains(t, out, "secret2")
			},
		},
		{
			name:  "list with prefix filter",
			opts:  list.Options{Prefix: "app/"},
			store: listStore([]string{"app/secret1", "other/secret2"}, nil, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "app/secret1")
				assert.NotContains(t, out, "other/secret2")
			},
		},
		{
			name:    "error from AWS",
			opts:    list.Options{},
			store:   listStore(nil, nil, nil, errors.New("AWS error")),
			wantErr: true,
		},
		{
			name:  "filter by regex",
			opts:  list.Options{Filter: "prod"},
			store: listStore([]string{"app/prod/secret1", "app/dev/secret2", "app/prod/secret3"}, nil, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "app/prod/secret1")
				assert.NotContains(t, out, "app/dev/secret2")
				assert.Contains(t, out, "app/prod/secret3")
			},
		},
		{
			name:    "invalid regex filter",
			opts:    list.Options{Filter: "[invalid"},
			store:   listStore(nil, nil, nil, nil),
			wantErr: true,
		},
		{
			name: "show secret values",
			opts: list.Options{Show: true},
			store: listStore(
				[]string{"secret1", "secret2"},
				map[string]string{"secret1": "value1", "secret2": "value2"},
				nil, nil,
			),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "secret1\tvalue1")
				assert.Contains(t, out, "secret2\tvalue2")
			},
		},
		{
			name:  "JSON output without show",
			opts:  list.Options{Output: output.FormatJSON},
			store: listStore([]string{"secret1", "secret2"}, nil, nil, nil),
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
			store: listStore(
				[]string{"secret1"},
				map[string]string{"secret1": "secret-value"},
				nil, nil,
			),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "secret1"`)
				assert.Contains(t, out, `"value": "secret-value"`)
			},
		},
		{
			name: "JSON output with show and error",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			store: listStore(
				[]string{"error-secret"}, nil,
				map[string]error{"error-secret": errors.New("access denied")}, nil,
			),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "error-secret"`)
				assert.Contains(t, out, `"error": "access denied"`)
			},
		},
		{
			name: "text output with error",
			opts: list.Options{Show: true},
			store: listStore(
				[]string{"error-secret"}, nil,
				map[string]error{"error-secret": errors.New("fetch error")}, nil,
			),
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
				UseCase: &secret.ListUseCase{Reader: tt.store},
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
