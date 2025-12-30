package diff

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
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
		name      string
		paramName string
		version1  string
		version2  string
		mock      *mockClient
		wantErr   bool
		check     func(t *testing.T, output string)
	}{
		{
			name:      "diff between two versions",
			paramName: "/app/param",
			version1:  "@1",
			version2:  "@2",
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					name := aws.ToString(params.Name)
					if name == "/app/param:1" {
						return &ssm.GetParameterOutput{
							Parameter: &types.Parameter{
								Name:             aws.String("/app/param"),
								Value:            aws.String("old-value"),
								Version:          1,
								Type:             types.ParameterTypeString,
								LastModifiedDate: aws.Time(now.Add(-time.Hour)),
							},
						}, nil
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:             aws.String("/app/param"),
							Value:            aws.String("new-value"),
							Version:          2,
							Type:             types.ParameterTypeString,
							LastModifiedDate: &now,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("-old-value")) {
					t.Error("expected '-old-value' in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new-value")) {
					t.Error("expected '+new-value' in diff output")
				}
			},
		},
		{
			name:      "no diff when same content",
			paramName: "/app/param",
			version1:  "@1",
			version2:  "@2",
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    aws.String("/app/param"),
							Value:   aws.String("same-value"),
							Version: 1,
							Type:    types.ParameterTypeString,
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// No diff lines expected for identical content
				if bytes.Contains([]byte(output), []byte("-same-value")) {
					t.Error("expected no diff for identical content")
				}
			},
		},
		{
			name:      "error getting first version",
			paramName: "/app/param",
			version1:  "@1",
			version2:  "@2",
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					if aws.ToString(params.Name) == "/app/param:1" {
						return nil, fmt.Errorf("version not found")
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    aws.String("/app/param"),
							Value:   aws.String("value"),
							Version: 2,
						},
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name:      "error getting second version",
			paramName: "/app/param",
			version1:  "@1",
			version2:  "@2",
			mock: &mockClient{
				getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
					if aws.ToString(params.Name) == "/app/param:2" {
						return nil, fmt.Errorf("version not found")
					}
					return &ssm.GetParameterOutput{
						Parameter: &types.Parameter{
							Name:    aws.String("/app/param"),
							Value:   aws.String("value"),
							Version: 1,
						},
					}, nil
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Run(context.Background(), tt.mock, &buf, tt.paramName, tt.version1, tt.version2)

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
