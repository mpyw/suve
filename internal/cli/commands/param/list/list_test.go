package list_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/list"
	"github.com/mpyw/suve/internal/cli/output"
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
	describeParametersFunc func(ctx context.Context, params *paramapi.DescribeParametersInput, optFns ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error)
	getParametersFunc      func(ctx context.Context, params *paramapi.GetParametersInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParametersOutput, error)
}

func (m *mockClient) DescribeParameters(ctx context.Context, params *paramapi.DescribeParametersInput, optFns ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
	if m.describeParametersFunc != nil {
		return m.describeParametersFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("DescribeParameters not mocked")
}

func (m *mockClient) GetParameters(ctx context.Context, params *paramapi.GetParametersInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParametersOutput, error) {
	if m.getParametersFunc != nil {
		return m.getParametersFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("GetParameters not mocked")
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
			name: "list all parameters",
			opts: list.Options{},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/param1")},
							{Name: lo.ToPtr("/app/param2")},
						},
					}, nil
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
				describeParametersFunc: func(_ context.Context, params *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					require.NotEmpty(t, params.ParameterFilters, "expected filter to be set")

					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/param1")},
						},
					}, nil
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
				describeParametersFunc: func(_ context.Context, params *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					require.NotEmpty(t, params.ParameterFilters, "expected filter to be set")
					assert.Equal(t, "Recursive", lo.FromPtr(params.ParameterFilters[0].Option))

					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/sub/param")},
						},
					}, nil
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
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "filter by regex",
			opts: list.Options{Filter: "prod"},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/prod/param1")},
							{Name: lo.ToPtr("/app/dev/param2")},
							{Name: lo.ToPtr("/app/prod/param3")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "/app/prod/param1")
				assert.NotContains(t, output, "/app/dev/param2")
				assert.Contains(t, output, "/app/prod/param3")
			},
		},
		{
			name: "invalid regex filter",
			opts: list.Options{Filter: "[invalid"},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return &paramapi.DescribeParametersOutput{}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "show parameter values",
			opts: list.Options{Show: true},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/param1")},
							{Name: lo.ToPtr("/app/param2")},
						},
					}, nil
				},
				getParametersFunc: func(_ context.Context, params *paramapi.GetParametersInput, _ ...func(*paramapi.Options)) (*paramapi.GetParametersOutput, error) {
					values := map[string]string{
						"/app/param1": "value1",
						"/app/param2": "value2",
					}

					var result []paramapi.Parameter

					for _, name := range params.Names {
						if val, ok := values[name]; ok {
							result = append(result, paramapi.Parameter{
								Name:  lo.ToPtr(name),
								Value: lo.ToPtr(val),
							})
						}
					}

					return &paramapi.GetParametersOutput{
						Parameters: result,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "/app/param1\tvalue1")
				assert.Contains(t, output, "/app/param2\tvalue2")
			},
		},
		{
			name: "JSON output without show",
			opts: list.Options{Output: output.FormatJSON},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/param1")},
							{Name: lo.ToPtr("/app/param2")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, `"name": "/app/param1"`)
				assert.Contains(t, out, `"name": "/app/param2"`)
				assert.NotContains(t, out, `"value"`)
			},
		},
		{
			name: "JSON output with show",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/param1")},
						},
					}, nil
				},
				getParametersFunc: func(_ context.Context, params *paramapi.GetParametersInput, _ ...func(*paramapi.Options)) (*paramapi.GetParametersOutput, error) {
					var result []paramapi.Parameter
					for _, name := range params.Names {
						result = append(result, paramapi.Parameter{
							Name:  lo.ToPtr(name),
							Value: lo.ToPtr("secret-value"),
						})
					}

					return &paramapi.GetParametersOutput{
						Parameters: result,
					}, nil
				},
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, `"name": "/app/param1"`)
				assert.Contains(t, out, `"value": "secret-value"`)
			},
		},
		{
			name: "JSON output with show and error",
			opts: list.Options{Output: output.FormatJSON, Show: true},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/error-param")},
						},
					}, nil
				},
				getParametersFunc: func(_ context.Context, params *paramapi.GetParametersInput, _ ...func(*paramapi.Options)) (*paramapi.GetParametersOutput, error) {
					// Return all requested parameters as invalid (simulates not found)
					return &paramapi.GetParametersOutput{
						InvalidParameters: params.Names,
					}, nil
				},
			},
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, `"name": "/app/error-param"`)
				assert.Contains(t, out, `"error":`)
			},
		},
		{
			name: "text output with error",
			opts: list.Options{Show: true},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
					return &paramapi.DescribeParametersOutput{
						Parameters: []paramapi.ParameterMetadata{
							{Name: lo.ToPtr("/app/error-param")},
						},
					}, nil
				},
				getParametersFunc: func(_ context.Context, params *paramapi.GetParametersInput, _ ...func(*paramapi.Options)) (*paramapi.GetParametersOutput, error) {
					// Return all requested parameters as invalid (simulates not found)
					return &paramapi.GetParametersOutput{
						InvalidParameters: params.Names,
					}, nil
				},
			},
			check: func(t *testing.T, out string) {
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
