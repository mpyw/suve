package list_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genericlist "github.com/mpyw/suve/internal/cli/commands/generic/list"
	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	ucparam "github.com/mpyw/suve/internal/usecase/param"
	ucsecret "github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Help(t *testing.T) {
	t.Parallel()

	t.Run("param", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()

		var buf bytes.Buffer

		app.Writer = &buf
		err := app.Run(t.Context(), []string{"suve", "param", "list", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "List parameters")
		assert.Contains(t, buf.String(), "--recursive")
		assert.Contains(t, buf.String(), "--filter")
		assert.Contains(t, buf.String(), "--show")
	})

	t.Run("secret", func(t *testing.T) {
		t.Parallel()

		app := apptest.AWSApp()

		var buf bytes.Buffer

		app.Writer = &buf
		err := app.Run(t.Context(), []string{"suve", "secret", "list", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "List secrets")
		assert.Contains(t, buf.String(), "--filter")
		assert.Contains(t, buf.String(), "--show")
	})
}

// paramListStore returns names via List and values (or errors) via Get for --show.
func paramListStore(names []string, values map[string]string, errNames map[string]bool) *providermock.Store {
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

// secretListStore mirrors paramListStore but supports typed per-name errors and a
// top-level List error.
func secretListStore(
	names []string, values map[string]string, errs map[string]error, listErr error,
) *providermock.Store {
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

// runParam runs the generic list Runner over the SSM Parameter Store usecase.
func runParam(
	t *testing.T, store *providermock.Store, input ucparam.ListInput, opts genericlist.Options,
) (string, error) {
	t.Helper()

	uc := &ucparam.ListUseCase{Reader: store}
	lister := func(ctx context.Context) ([]genericlist.Entry, error) {
		result, err := uc.Execute(ctx, input)
		if err != nil {
			return nil, err
		}

		entries := make([]genericlist.Entry, len(result.Entries))
		for i, e := range result.Entries {
			entries[i] = genericlist.Entry{Name: e.Name, Value: e.Value, Error: e.Error}
		}

		return entries, nil
	}

	var buf, errBuf bytes.Buffer

	r := &genericlist.Runner{List: lister, Options: opts, Stdout: &buf, Stderr: &errBuf}
	err := r.Run(t.Context())

	return buf.String(), err
}

// runSecret runs the generic list Runner over the Secrets Manager usecase.
func runSecret(
	t *testing.T, store *providermock.Store, input ucsecret.ListInput, opts genericlist.Options,
) (string, error) {
	t.Helper()

	uc := &ucsecret.ListUseCase{Reader: store}
	lister := func(ctx context.Context) ([]genericlist.Entry, error) {
		result, err := uc.Execute(ctx, input)
		if err != nil {
			return nil, err
		}

		entries := make([]genericlist.Entry, len(result.Entries))
		for i, e := range result.Entries {
			entries[i] = genericlist.Entry{Name: e.Name, Value: e.Value, Error: e.Error}
		}

		return entries, nil
	}

	var buf, errBuf bytes.Buffer

	r := &genericlist.Runner{List: lister, Options: opts, Stdout: &buf, Stderr: &errBuf}
	err := r.Run(t.Context())

	return buf.String(), err
}

func TestRunParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   ucparam.ListInput
		opts    genericlist.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name:  "list all parameters",
			store: paramListStore([]string{"/app/param1", "/app/param2"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1")
				assert.Contains(t, output, "/app/param2")
			},
		},
		{
			name:  "list with prefix",
			input: ucparam.ListInput{Prefix: "/app/"},
			store: paramListStore([]string{"/app/param1"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1")
			},
		},
		{
			name:  "recursive listing",
			input: ucparam.ListInput{Prefix: "/app/", Recursive: true},
			store: paramListStore([]string{"/app/sub/param"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/sub/param")
			},
		},
		{
			name:  "prefix uses path hierarchy (does not over-match)",
			input: ucparam.ListInput{Prefix: "/app", Recursive: true},
			store: paramListStore([]string{"/app/param1", "/application/other"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1")
				assert.NotContains(t, output, "/application/other")
			},
		},
		{
			name: "error from AWS",
			store: &providermock.Store{
				ListFunc: func(_ context.Context) ([]string, error) {
					return nil, assert.AnError
				},
			},
			wantErr: true,
		},
		{
			name:  "filter by regex",
			input: ucparam.ListInput{Filter: "prod"},
			store: paramListStore([]string{"/app/prod/param1", "/app/dev/param2", "/app/prod/param3"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/prod/param1")
				assert.NotContains(t, output, "/app/dev/param2")
				assert.Contains(t, output, "/app/prod/param3")
			},
		},
		{
			name:    "invalid regex filter",
			input:   ucparam.ListInput{Filter: "[invalid"},
			store:   paramListStore(nil, nil, nil),
			wantErr: true,
		},
		{
			name:  "show parameter values",
			opts:  genericlist.Options{Show: true},
			input: ucparam.ListInput{WithValue: true},
			store: paramListStore(
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
			opts:  genericlist.Options{Output: output.FormatJSON},
			store: paramListStore([]string{"/app/param1", "/app/param2"}, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "/app/param1"`)
				assert.Contains(t, out, `"name": "/app/param2"`)
				assert.NotContains(t, out, `"value"`)
			},
		},
		{
			name:  "JSON output with show",
			opts:  genericlist.Options{Output: output.FormatJSON, Show: true},
			input: ucparam.ListInput{WithValue: true},
			store: paramListStore(
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
			name:  "JSON output with show and error",
			opts:  genericlist.Options{Output: output.FormatJSON, Show: true},
			input: ucparam.ListInput{WithValue: true},
			store: paramListStore(
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
			name:  "text output with error",
			opts:  genericlist.Options{Show: true},
			input: ucparam.ListInput{WithValue: true},
			store: paramListStore(
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

			out, err := runParam(t, tt.store, tt.input, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestRunSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   ucsecret.ListInput
		opts    genericlist.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name:  "list all secrets",
			store: secretListStore([]string{"secret1", "secret2"}, nil, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "secret1")
				assert.Contains(t, out, "secret2")
			},
		},
		{
			name:  "list with prefix filter",
			input: ucsecret.ListInput{Prefix: "app/"},
			store: secretListStore([]string{"app/secret1", "other/secret2"}, nil, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "app/secret1")
				assert.NotContains(t, out, "other/secret2")
			},
		},
		{
			name:    "error from AWS",
			store:   secretListStore(nil, nil, nil, errors.New("AWS error")),
			wantErr: true,
		},
		{
			name:  "filter by regex",
			input: ucsecret.ListInput{Filter: "prod"},
			store: secretListStore([]string{"app/prod/secret1", "app/dev/secret2", "app/prod/secret3"}, nil, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "app/prod/secret1")
				assert.NotContains(t, out, "app/dev/secret2")
				assert.Contains(t, out, "app/prod/secret3")
			},
		},
		{
			name:    "invalid regex filter",
			input:   ucsecret.ListInput{Filter: "[invalid"},
			store:   secretListStore(nil, nil, nil, nil),
			wantErr: true,
		},
		{
			name:  "show secret values",
			opts:  genericlist.Options{Show: true},
			input: ucsecret.ListInput{WithValue: true},
			store: secretListStore(
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
			opts:  genericlist.Options{Output: output.FormatJSON},
			store: secretListStore([]string{"secret1", "secret2"}, nil, nil, nil),
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "secret1"`)
				assert.Contains(t, out, `"name": "secret2"`)
				assert.NotContains(t, out, `"value"`)
			},
		},
		{
			name:  "JSON output with show",
			opts:  genericlist.Options{Output: output.FormatJSON, Show: true},
			input: ucsecret.ListInput{WithValue: true},
			store: secretListStore(
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
			name:  "JSON output with show and error",
			opts:  genericlist.Options{Output: output.FormatJSON, Show: true},
			input: ucsecret.ListInput{WithValue: true},
			store: secretListStore(
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
			name:  "text output with error",
			opts:  genericlist.Options{Show: true},
			input: ucsecret.ListInput{WithValue: true},
			store: secretListStore(
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

			out, err := runSecret(t, tt.store, tt.input, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}
