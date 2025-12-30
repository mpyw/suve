package ls

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
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
	tests := []struct {
		name      string
		prefix    string
		recursive bool
		mock      *mockClient
		wantErr   bool
		check     func(t *testing.T, output string)
	}{
		{
			name:   "list all parameters",
			prefix: "",
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, _ *ssm.DescribeParametersInput, _ ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
					return &ssm.DescribeParametersOutput{
						Parameters: []types.ParameterMetadata{
							{Name: aws.String("/app/param1")},
							{Name: aws.String("/app/param2")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("/app/param1")) {
					t.Error("expected /app/param1 in output")
				}
				if !bytes.Contains([]byte(output), []byte("/app/param2")) {
					t.Error("expected /app/param2 in output")
				}
			},
		},
		{
			name:   "list with prefix",
			prefix: "/app/",
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, params *ssm.DescribeParametersInput, _ ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
					if len(params.ParameterFilters) == 0 {
						t.Error("expected filter to be set")
					}
					return &ssm.DescribeParametersOutput{
						Parameters: []types.ParameterMetadata{
							{Name: aws.String("/app/param1")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("/app/param1")) {
					t.Error("expected /app/param1 in output")
				}
			},
		},
		{
			name:      "recursive listing",
			prefix:    "/app/",
			recursive: true,
			mock: &mockClient{
				describeParametersFunc: func(_ context.Context, params *ssm.DescribeParametersInput, _ ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
					if len(params.ParameterFilters) == 0 {
						t.Error("expected filter to be set")
						return nil, nil
					}
					if aws.ToString(params.ParameterFilters[0].Option) != "Recursive" {
						t.Errorf("expected Recursive option, got %s", aws.ToString(params.ParameterFilters[0].Option))
					}
					return &ssm.DescribeParametersOutput{
						Parameters: []types.ParameterMetadata{
							{Name: aws.String("/app/sub/param")},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("/app/sub/param")) {
					t.Error("expected /app/sub/param in output")
				}
			},
		},
		{
			name: "error from AWS",
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
			var buf bytes.Buffer
			err := Run(t.Context(), tt.mock, &buf, tt.prefix, tt.recursive)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
