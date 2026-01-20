package list_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/list"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/model"
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

type mockClient struct {
	listParametersResult []*model.ParameterListItem
	listParametersErr    error
	getParameterValues   map[string]string
	getParameterErr      map[string]error
}

func (m *mockClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	if m.listParametersErr != nil {
		return nil, m.listParametersErr
	}

	return m.listParametersResult, nil
}

func (m *mockClient) GetParameter(_ context.Context, name, _ string) (*model.Parameter, error) {
	if m.getParameterErr != nil {
		if err, ok := m.getParameterErr[name]; ok {
			return nil, err
		}
	}

	if m.getParameterValues != nil {
		if value, ok := m.getParameterValues[name]; ok {
			return &model.Parameter{Name: name, Value: value}, nil
		}
	}

	return nil, errors.New("not found")
}

func (m *mockClient) GetParameterHistory(_ context.Context, _ string) (*model.ParameterHistory, error) {
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
			name: "list all parameters",
			opts: list.Options{},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/param1"},
					{Name: "/app/param2"},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1")
				assert.Contains(t, output, "/app/param2")
			},
		},
		{
			name: "list with prefix",
			opts: list.Options{Prefix: "/app/"},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/param1"},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1")
			},
		},
		{
			name: "recursive listing",
			opts: list.Options{Prefix: "/app/", Recursive: true},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/sub/param"},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/sub/param")
			},
		},
		{
			name: "error from AWS",
			opts: list.Options{},
			mock: &mockClient{
				listParametersErr: errors.New("AWS error"),
			},
			wantErr: true,
		},
		{
			name: "filter by regex",
			opts: list.Options{Filter: "prod"},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/prod/param1"},
					{Name: "/app/dev/param2"},
					{Name: "/app/prod/param3"},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/prod/param1")
				assert.NotContains(t, output, "/app/dev/param2")
				assert.Contains(t, output, "/app/prod/param3")
			},
		},
		{
			name: "invalid regex filter",
			opts: list.Options{Filter: "[invalid"},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{},
			},
			wantErr: true,
		},
		{
			name: "show parameter values",
			opts: list.Options{Show: true},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/param1"},
					{Name: "/app/param2"},
				},
				getParameterValues: map[string]string{
					"/app/param1": "value1",
					"/app/param2": "value2",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "/app/param1\tvalue1")
				assert.Contains(t, output, "/app/param2\tvalue2")
			},
		},
		{
			name: "JSON output without show",
			opts: list.Options{Output: output.FormatJSON},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/param1"},
					{Name: "/app/param2"},
				},
			},
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
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/param1"},
				},
				getParameterValues: map[string]string{
					"/app/param1": "secret-value",
				},
			},
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "/app/param1"`)
				assert.Contains(t, out, `"value": "secret-value"`)
			},
		},
		{
			name: "JSON output with show and error",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/error-param"},
				},
				getParameterErr: map[string]error{
					"/app/error-param": errors.New("parameter not found"),
				},
			},
			check: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "/app/error-param"`)
				assert.Contains(t, out, `"error":`)
			},
		},
		{
			name: "text output with error",
			opts: list.Options{Show: true},
			mock: &mockClient{
				listParametersResult: []*model.ParameterListItem{
					{Name: "/app/error-param"},
				},
				getParameterErr: map[string]error{
					"/app/error-param": errors.New("fetch error"),
				},
			},
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
				UseCase: &param.ListUseCase{Client: tt.mock},
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
