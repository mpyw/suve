package log

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
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
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
		name       string
		paramName  string
		maxResults int32
		showPatch  bool
		mock       *mockClient
		wantErr    bool
		check      func(t *testing.T, output string)
	}{
		{
			name:       "show history",
			paramName:  "/app/param",
			maxResults: 10,
			showPatch:  false,
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, params *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					if aws.ToString(params.Name) != "/app/param" {
						t.Errorf("expected name /app/param, got %s", aws.ToString(params.Name))
					}
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: aws.String("/app/param"), Value: aws.String("v1"), Version: 1, LastModifiedDate: aws.Time(now.Add(-time.Hour))},
							{Name: aws.String("/app/param"), Value: aws.String("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("Version 2")) {
					t.Error("expected Version 2 in output")
				}
				if !bytes.Contains([]byte(output), []byte("Version 1")) {
					t.Error("expected Version 1 in output")
				}
				if !bytes.Contains([]byte(output), []byte("(current)")) {
					t.Error("expected (current) label in output")
				}
			},
		},
		{
			name:       "truncate long values",
			paramName:  "/app/param",
			maxResults: 10,
			showPatch:  false,
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					longValue := "this is a very long value that should be truncated in the preview"
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: aws.String("/app/param"), Value: aws.String(longValue), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if !bytes.Contains([]byte(output), []byte("...")) {
					t.Error("expected truncation indicator in output")
				}
			},
		},
		{
			name:       "show patch between versions",
			paramName:  "/app/param",
			maxResults: 10,
			showPatch:  true,
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: aws.String("/app/param"), Value: aws.String("old-value"), Version: 1, LastModifiedDate: aws.Time(now.Add(-time.Hour))},
							{Name: aws.String("/app/param"), Value: aws.String("new-value"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Should contain diff markers
				if !bytes.Contains([]byte(output), []byte("-old-value")) {
					t.Error("expected -old-value in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new-value")) {
					t.Error("expected +new-value in diff output")
				}
				// Should contain version headers
				if !bytes.Contains([]byte(output), []byte("/app/param#1")) {
					t.Error("expected /app/param#1 in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("/app/param#2")) {
					t.Error("expected /app/param#2 in diff output")
				}
			},
		},
		{
			name:       "patch with single version shows no diff",
			paramName:  "/app/param",
			maxResults: 10,
			showPatch:  true,
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: aws.String("/app/param"), Value: aws.String("only-value"), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Should contain version info but no diff markers
				if !bytes.Contains([]byte(output), []byte("Version 1")) {
					t.Error("expected Version 1 in output")
				}
				// No diff markers for single version
				if bytes.Contains([]byte(output), []byte("---")) {
					t.Error("expected no diff markers for single version")
				}
			},
		},
		{
			name:       "error from AWS",
			paramName:  "/app/param",
			maxResults: 10,
			showPatch:  false,
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf, errBuf bytes.Buffer
			err := Run(t.Context(), tt.mock, &buf, &errBuf, tt.paramName, tt.maxResults, tt.showPatch, false)

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
