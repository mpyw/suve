package list_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/list"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Help(t *testing.T) {
	t.Parallel()

	app := appcli.MakeApp()

	var buf bytes.Buffer

	app.Writer = &buf
	err := app.Run(t.Context(), []string{"suve", "param", "list", "--help"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "List parameters")
	assert.Contains(t, buf.String(), "--recursive")
	assert.Contains(t, buf.String(), "--filter")
	assert.Contains(t, buf.String(), "--show")
}

// listStore returns names via List and values (or errors) via Get for --show.
func listStore(names []string, values map[string]string, errNames map[string]bool) *providermock.Store {
	return &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return names, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			if errNames[name] {
				return nil, assert.AnError
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
			name:  "list all parameters",
			opts:  list.Options{},
			store: listStore([]string{"/app/param1", "/app/param2"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1")
				assert.Contains(t, output, "/app/param2")
			},
		},
		{
			name:  "list with prefix",
			opts:  list.Options{Prefix: "/app/"},
			store: listStore([]string{"/app/param1"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1")
			},
		},
		{
			name:  "recursive listing",
			opts:  list.Options{Prefix: "/app/", Recursive: true},
			store: listStore([]string{"/app/sub/param"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/sub/param")
			},
		},
		{
			name:  "prefix uses path hierarchy (does not over-match)",
			opts:  list.Options{Prefix: "/app", Recursive: true},
			store: listStore([]string{"/app/param1", "/application/other"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1")
				assert.NotContains(t, output, "/application/other")
			},
		},
		{
			name: "error from AWS",
			opts: list.Options{},
			store: &providermock.Store{
				ListFunc: func(_ context.Context) ([]string, error) {
					return nil, assert.AnError
				},
			},
			wantErr: true,
		},
		{
			name:  "filter by regex",
			opts:  list.Options{Filter: "prod"},
			store: listStore([]string{"/app/prod/param1", "/app/dev/param2", "/app/prod/param3"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/prod/param1")
				assert.NotContains(t, output, "/app/dev/param2")
				assert.Contains(t, output, "/app/prod/param3")
			},
		},
		{
			name:    "invalid regex filter",
			opts:    list.Options{Filter: "[invalid"},
			store:   listStore(nil, nil, nil),
			wantErr: true,
		},
		{
			name: "show parameter values",
			opts: list.Options{Show: true},
			store: listStore(
				[]string{"/app/param1", "/app/param2"},
				map[string]string{"/app/param1": "value1", "/app/param2": "value2"},
				nil,
			),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1\tvalue1")
				assert.Contains(t, output, "/app/param2\tvalue2")
			},
		},
		{
			name:  "JSON output without show",
			opts:  list.Options{Output: output.FormatJSON},
			store: listStore([]string{"/app/param1", "/app/param2"}, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "/app/param1"`)
				assert.Contains(t, out, `"name": "/app/param2"`)
				assert.NotContains(t, out, `"value"`)
			},
		},
		{
			name: "JSON output with show",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			store: listStore(
				[]string{"/app/param1"},
				map[string]string{"/app/param1": "secret-value"},
				nil,
			),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "/app/param1"`)
				assert.Contains(t, out, `"value": "secret-value"`)
			},
		},
		{
			name: "JSON output with show and error",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			store: listStore(
				[]string{"/app/error-param"},
				nil,
				map[string]bool{"/app/error-param": true},
			),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "/app/error-param"`)
				assert.Contains(t, out, `"error":`)
			},
		},
		{
			name: "text output with error",
			opts: list.Options{Show: true},
			store: listStore(
				[]string{"/app/error-param"},
				nil,
				map[string]bool{"/app/error-param": true},
			),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "/app/error-param")
				assert.Contains(t, out, "<error:")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			r := &list.Runner{
				UseCase: &param.ListUseCase{Reader: tt.store},
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
