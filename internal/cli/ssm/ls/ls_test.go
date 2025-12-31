package ls_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/ssm/ls"
)

type mockClient struct {
	describeParametersFunc func(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
}

func (m *mockClient) DescribeParameters(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
	if m.describeParametersFunc != nil {
		return m.describeParametersFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("DescribeParameters not mocked")
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
			name: "list all parameters",
			opts: ls.Options{},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *ssm.DescribeParametersInput, _ ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
					return &ssm.DescribeParametersOutput{
						Parameters: []types.ParameterMetadata{
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
			opts: ls.Options{Prefix: "/app/"},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, params *ssm.DescribeParametersInput, _ ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
					require.NotEmpty(t, params.ParameterFilters, "expected filter to be set")
					return &ssm.DescribeParametersOutput{
						Parameters: []types.ParameterMetadata{
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
			opts: ls.Options{Prefix: "/app/", Recursive: true},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, params *ssm.DescribeParametersInput, _ ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
					require.NotEmpty(t, params.ParameterFilters, "expected filter to be set")
					assert.Equal(t, "Recursive", lo.FromPtr(params.ParameterFilters[0].Option))
					return &ssm.DescribeParametersOutput{
						Parameters: []types.ParameterMetadata{
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
			opts: ls.Options{},
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *ssm.DescribeParametersInput, _ ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
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
