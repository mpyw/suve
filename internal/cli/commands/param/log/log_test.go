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

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/log"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "log"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve param log")
	})

	t.Run("invalid from version", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "log", "--from", "invalid", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --from value")
	})

	t.Run("invalid to version", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "log", "--to", "invalid", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --to value")
	})

	t.Run("from with shift syntax not allowed", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "log", "--from", "~1", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shift syntax (~) not supported")
	})

	t.Run("to with shift syntax not allowed", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "log", "--to", "~1", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shift syntax (~) not supported")
	})

	t.Run("from without version specifier", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "log", "--from", "/app/param", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version specifier required")
	})

	t.Run("to without version specifier", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "log", "--to", "/app/param", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version specifier required")
	})

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "ssm", "log", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Show parameter version history")
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
		{
			name: "oneline format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Oneline: true},
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
				// Oneline format should be compact
				assert.Contains(t, output, "2")
				assert.Contains(t, output, "(current)")
				assert.Contains(t, output, "v2")
				// Should not have "Version" prefix like normal format
				assert.NotContains(t, output, "Version 2")
			},
		},
		{
			name: "oneline truncates long values",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Oneline: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					longValue := "this is a very long value that exceeds forty characters"
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr(longValue), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "...")
				// Full value should not appear
				assert.NotContains(t, output, "exceeds forty characters")
			},
		},
		{
			name: "filter by from version",
			opts: log.Options{Name: "/app/param", MaxResults: 10, FromVersion: lo.ToPtr(int64(2))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version 2")
				assert.Contains(t, output, "Version 3")
				assert.NotContains(t, output, "Version 1")
			},
		},
		{
			name: "filter by to version",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ToVersion: lo.ToPtr(int64(2))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version 1")
				assert.Contains(t, output, "Version 2")
				assert.NotContains(t, output, "Version 3")
			},
		},
		{
			name: "filter by from and to version range",
			opts: log.Options{Name: "/app/param", MaxResults: 10, FromVersion: lo.ToPtr(int64(2)), ToVersion: lo.ToPtr(int64(4))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-3 * time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v4"), Version: 4, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.NotContains(t, output, "Version 1")
				assert.Contains(t, output, "Version 2")
				assert.Contains(t, output, "Version 3")
				assert.Contains(t, output, "Version 4")
			},
		},
		{
			name: "filter with no matching versions returns empty",
			opts: log.Options{Name: "/app/param", MaxResults: 10, FromVersion: lo.ToPtr(int64(10)), ToVersion: lo.ToPtr(int64(20))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
					return &ssm.GetParameterHistoryOutput{
						Parameters: []types.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: &now},
						},
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
