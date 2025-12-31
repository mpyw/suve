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
		name      string
		paramName string
		opts      Options
		mock      *mockClient
		wantErr   bool
		check     func(t *testing.T, output string)
	}{
		{
			name:      "show history",
			paramName: "/app/param",
			opts:      Options{MaxResults: 10},
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
			name:      "truncate long values",
			paramName: "/app/param",
			opts:      Options{MaxResults: 10},
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
			name:      "show patch between versions",
			paramName: "/app/param",
			opts:      Options{MaxResults: 10, ShowPatch: true},
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
			name:      "patch with single version shows no diff",
			paramName: "/app/param",
			opts:      Options{MaxResults: 10, ShowPatch: true},
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
			name:      "error from AWS",
			paramName: "/app/param",
			opts:      Options{MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name:      "reverse order shows oldest first",
			paramName: "/app/param",
			opts:      Options{MaxResults: 10, Reverse: true},
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
			check: func(t *testing.T, output string) {
				// In reverse mode, Version 1 should appear before Version 2
				v1Pos := bytes.Index([]byte(output), []byte("Version 1"))
				v2Pos := bytes.Index([]byte(output), []byte("Version 2"))
				if v1Pos < 0 || v2Pos < 0 {
					t.Error("expected both versions in output")
				}
				if v1Pos > v2Pos {
					t.Error("expected Version 1 before Version 2 in reverse mode")
				}
				// (current) should be on Version 2 (last in list)
				currentPos := bytes.Index([]byte(output), []byte("(current)"))
				if currentPos < v2Pos {
					t.Error("expected (current) label on Version 2 in reverse mode")
				}
			},
		},
		{
			name:      "reverse with patch shows diff correctly",
			paramName: "/app/param",
			opts:      Options{MaxResults: 10, ShowPatch: true, Reverse: true},
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
				// In reverse mode with patch, diff should show old->new transition
				if !bytes.Contains([]byte(output), []byte("-old-value")) {
					t.Error("expected -old-value in diff output")
				}
				if !bytes.Contains([]byte(output), []byte("+new-value")) {
					t.Error("expected +new-value in diff output")
				}
				// Diff should be under Version 1 (first in reverse mode), comparing #1 -> #2
				if !bytes.Contains([]byte(output), []byte("--- /app/param#1")) {
					t.Error("expected diff header '--- /app/param#1'")
				}
				if !bytes.Contains([]byte(output), []byte("+++ /app/param#2")) {
					t.Error("expected diff header '+++ /app/param#2'")
				}
			},
		},
		{
			name:      "empty history",
			paramName: "/app/param",
			opts:      Options{MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				if output != "" {
					t.Errorf("expected empty output for empty history, got: %s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf, errBuf bytes.Buffer
			err := Run(t.Context(), tt.mock, &buf, &errBuf, tt.paramName, tt.opts)

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
