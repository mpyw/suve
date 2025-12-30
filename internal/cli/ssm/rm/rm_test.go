package rm

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type mockClient struct {
	deleteParameterFunc func(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
}

func (m *mockClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("DeleteParameter not mocked")
}

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		paramName string
		mock      *mockClient
		wantErr   bool
		check     func(t *testing.T, output string)
	}{
		{
			name:      "delete parameter",
			paramName: "/app/param",
			mock: &mockClient{
				deleteParameterFunc: func(_ context.Context, _ *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
					return &ssm.DeleteParameterOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Deleted")) {
					t.Error("expected 'Deleted' in output")
				}
				if !bytes.Contains([]byte(output), []byte("/app/param")) {
					t.Error("expected parameter name in output")
				}
			},
		},
		{
			name:      "error from AWS",
			paramName: "/app/param",
			mock: &mockClient{
				deleteParameterFunc: func(_ context.Context, _ *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(context.Background(), tt.mock, &buf, tt.paramName)

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
