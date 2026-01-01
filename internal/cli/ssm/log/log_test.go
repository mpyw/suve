package log_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	"github.com/mpyw/suve/internal/cli/ssm/log"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "log"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve ssm log")
	})
}

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
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name    string
		opts    log.Options
		mock    *mockClient
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show history",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, params *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version 2")
				assert.Contains(t, output, "Version 1")
				assert.Contains(t, output, "(current)")
			},
		},
		{
			name: "truncate long values",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					longValue := "this is a very long value that should be truncated in the preview"
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr(longValue), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "...")
			},
		},
		{
			name: "show patch between versions",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("old-value"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("new-value"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-old-value")
				assert.Contains(t, output, "+new-value")
				assert.Contains(t, output, "/app/param#1")
				assert.Contains(t, output, "/app/param#2")
			},
		},
		{
			name: "patch with single version shows no diff",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("only-value"), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version 1")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name: "error from AWS",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Reverse: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				v1Pos := bytes.Index([]byte(output), []byte("Version 1"))
				v2Pos := bytes.Index([]byte(output), []byte("Version 2"))
				require.NotEqual(t, -1, v1Pos, "expected Version 1 in output")
				require.NotEqual(t, -1, v2Pos, "expected Version 2 in output")
				assert.Less(t, v1Pos, v2Pos, "expected Version 1 before Version 2 in reverse mode")
				currentPos := bytes.Index([]byte(output), []byte("(current)"))
				assert.Greater(t, currentPos, v2Pos, "expected (current) label after Version 2 in reverse mode")
			},
		},
		{
			name: "reverse with patch shows diff correctly",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true, Reverse: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("old-value"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("new-value"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-old-value")
				assert.Contains(t, output, "+new-value")
				assert.Contains(t, output, "--- /app/param#1")
				assert.Contains(t, output, "+++ /app/param#2")
			},
		},
		{
			name: "empty history",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Empty(t, output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &log.Runner{
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
