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
)

func TestCommand_Help(t *testing.T) {
	t.Parallel()
	app := appcli.MakeApp()
	var buf bytes.Buffer
	app.Writer = &buf
	err := app.Run(context.Background(), []string{"suve", "param", "list", "--help"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "List parameters")
	assert.Contains(t, buf.String(), "--recursive")
}

type mockClient struct {
	describeParametersFunc func(ctx context.Context, params *paramapi.DescribeParametersInput, optFns ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error)
}

func (m *mockClient) DescribeParameters(ctx context.Context, params *paramapi.DescribeParametersInput, optFns ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
	if m.describeParametersFunc != nil {
		return m.describeParametersFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("DescribeParameters not mocked")
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
