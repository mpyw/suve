package cat

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/mpyw/suve/internal/version"
)

type mockClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

func TestRun(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		spec    *version.Spec
		mock    *mockClient
		decrypt bool
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "output raw value",
			spec: &version.Spec{Name: "/app/param"},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             aws.String("/app/param"),
							Value:            aws.String("raw-value"),
							Version:          1,
							Type:             types.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			decrypt: true,
			check: func(t *testing.T, output string) {
				// cat outputs raw value without newline or decoration
				if output != "raw-value" {
					t.Errorf("expected 'raw-value', got %q", output)
				}
			},
		},
		{
			name: "output with shift",
			spec: &version.Spec{Name: "/app/param", Shift: 1},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: aws.String("/app/param"), Value: aws.String("v1"), Version: 1, LastModifiedDate: aws.Time(now.Add(-time.Hour))},
							{Name: aws.String("/app/param"), Value: aws.String("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			decrypt: true,
			check: func(t *testing.T, output string) {
				if output != "v1" {
					t.Errorf("expected 'v1', got %q", output)
				}
			},
		},
		{
			name: "error from AWS",
			spec: &version.Spec{Name: "/app/param"},
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			decrypt: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(t.Context(), tt.mock, &buf, tt.spec, tt.decrypt)

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
