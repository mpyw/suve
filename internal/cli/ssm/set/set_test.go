package set

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
	putParameterFunc func(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

func (m *mockClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("PutParameter not mocked")
}

func TestRun(t *testing.T) {
	tests := []struct {
		name        string
		paramName   string
		value       string
		paramType   string
		description string
		mock        *mockClient
		wantErr     bool
		check       func(t *testing.T, output string)
	}{
		{
			name:      "set parameter",
			paramName: "/app/param",
			value:     "test-value",
			paramType: "SecureString",
			mock: &mockClient{
				putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
					if aws.ToString(params.Name) != "/app/param" {
						t.Errorf("expected name /app/param, got %s", aws.ToString(params.Name))
					}
					if aws.ToString(params.Value) != "test-value" {
						t.Errorf("expected value test-value, got %s", aws.ToString(params.Value))
					}
					if params.Type != types.ParameterTypeSecureString {
						t.Errorf("expected type SecureString, got %s", params.Type)
					}
					return &ssm.PutParameterOutput{
						Version: 1,
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("/app/param")) {
					t.Error("expected parameter name in output")
				}
				if !bytes.Contains([]byte(output), []byte("version: 1")) {
					t.Error("expected version in output")
				}
			},
		},
		{
			name:        "set with description",
			paramName:   "/app/param",
			value:       "test-value",
			paramType:   "String",
			description: "Test description",
			mock: &mockClient{
				putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
					if aws.ToString(params.Description) != "Test description" {
						t.Errorf("expected description 'Test description', got %s", aws.ToString(params.Description))
					}
					return &ssm.PutParameterOutput{
						Version: 1,
					}, nil
				},
			},
		},
		{
			name:      "error from AWS",
			paramName: "/app/param",
			value:     "test-value",
			paramType: "String",
			mock: &mockClient{
				putParameterFunc: func(_ context.Context, _ *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(t.Context(), tt.mock, &buf, tt.paramName, tt.value, tt.paramType, tt.description)

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
