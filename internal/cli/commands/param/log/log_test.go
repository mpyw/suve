package log_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/log"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "log"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve param log")
	})

	t.Run("invalid since timestamp", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "log", "--since", "invalid", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --since value")
	})

	t.Run("invalid until timestamp", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "log", "--until", "invalid", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --until value")
	})

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(t.Context(), []string{"suve", "param", "log", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Show parameter version history")
	})

	t.Run("json without patch warns", func(t *testing.T) {
		t.Parallel()
		var errBuf bytes.Buffer
		app := appcli.MakeApp()
		app.ErrWriter = &errBuf
		_ = app.Run(t.Context(), []string{"suve", "param", "log", "--parse-json", "/app/param"})
		assert.Contains(t, errBuf.String(), "--parse-json has no effect")
	})

	t.Run("oneline with patch warns", func(t *testing.T) {
		t.Parallel()
		var errBuf bytes.Buffer
		app := appcli.MakeApp()
		app.ErrWriter = &errBuf
		_ = app.Run(t.Context(), []string{"suve", "param", "log", "--oneline", "--patch", "/app/param"})
		assert.Contains(t, errBuf.String(), "--oneline has no effect")
	})

	t.Run("output json with patch warns", func(t *testing.T) {
		t.Parallel()
		var errBuf bytes.Buffer
		app := appcli.MakeApp()
		app.ErrWriter = &errBuf
		_ = app.Run(t.Context(), []string{"suve", "param", "log", "--output=json", "--patch", "/app/param"})
		assert.Contains(t, errBuf.String(), "-p/--patch has no effect")
	})

	t.Run("output json with oneline warns", func(t *testing.T) {
		t.Parallel()
		var errBuf bytes.Buffer
		app := appcli.MakeApp()
		app.ErrWriter = &errBuf
		_ = app.Run(t.Context(), []string{"suve", "param", "log", "--output=json", "--oneline", "/app/param"})
		assert.Contains(t, errBuf.String(), "--oneline has no effect")
	})
}

type mockClient struct {
	getParameterHistoryFunc func(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error)
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

//nolint:funlen // Table-driven test with many cases
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
				getParameterHistoryFunc: func(_ context.Context, params *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					assert.Equal(t, "/app/param", lo.FromPtr(params.Name))
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 2")
				assert.Contains(t, output, "Version 1")
				assert.Contains(t, output, "(current)")
			},
		},
		{
			name: "normal mode shows full value without truncation",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					longValue := "this is a very long value that should NOT be truncated in normal mode"
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr(longValue), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// Normal mode shows full value without truncation
				assert.Contains(t, output, "should NOT be truncated in normal mode")
				assert.NotContains(t, output, "...")
			},
		},
		{
			name: "max-value-length truncates in normal mode",
			opts: log.Options{Name: "/app/param", MaxResults: 10, MaxValueLength: 20},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					longValue := "this is a very long value that should be truncated"
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr(longValue), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "...")
				// Full value should not appear
				assert.NotContains(t, output, "should be truncated")
			},
		},
		{
			name: "show patch between versions",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("old-value"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("new-value"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
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
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("only-value"), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 1")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name: "error from AWS",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Reverse: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				v1Pos := strings.Index(output, "Version 1")
				v2Pos := strings.Index(output, "Version 2")
				require.NotEqual(t, -1, v1Pos, "expected Version 1 in output")
				require.NotEqual(t, -1, v2Pos, "expected Version 2 in output")
				assert.Less(t, v1Pos, v2Pos, "expected Version 1 before Version 2 in reverse mode")
				currentPos := strings.Index(output, "(current)")
				assert.Greater(t, currentPos, v2Pos, "expected (current) label after Version 2 in reverse mode")
			},
		},
		{
			name: "reverse with patch shows diff correctly",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true, Reverse: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
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
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{},
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
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
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
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					longValue := "this is a very long value that exceeds forty characters"
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
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
			name: "filter by since date",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-90 * time.Minute))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
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
			name: "filter by until date",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
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
			name: "filter by since and until date range",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-150 * time.Minute)), Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
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
				assert.NotContains(t, output, "Version 4")
			},
		},
		{
			name: "filter with no matching dates returns empty",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(time.Hour)), Until: lo.ToPtr(now.Add(2 * time.Hour))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Empty(t, output)
			},
		},
		{
			name: "filter skips versions without LastModifiedDate",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: nil},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version 2")
				assert.NotContains(t, output, "Version 1")
			},
		},
		{
			name: "JSON output format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Output: output.FormatJSON},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("value1"), Version: 1, Type: paramapi.ParameterTypeString, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("value2"), Version: 2, Type: paramapi.ParameterTypeSecureString, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, `"version"`)
				assert.Contains(t, output, `"value"`)
				assert.Contains(t, output, `"type"`)
				assert.Contains(t, output, `"SecureString"`)
			},
		},
		{
			name: "JSON output without LastModifiedDate",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Output: output.FormatJSON},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("value1"), Version: 1, Type: paramapi.ParameterTypeString, LastModifiedDate: nil},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, `"version"`)
				assert.NotContains(t, output, `"modified"`)
			},
		},
		{
			name: "patch with JSON format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr(`{"key":"old"}`), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr(`{"key":"new"}`), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "version without LastModifiedDate shows correctly",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: nil},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version 1")
				assert.NotContains(t, output, "Date:")
			},
		},
		{
			name: "oneline without LastModifiedDate",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Oneline: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: nil},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "1")
			},
		},
		{
			name: "patch with identical values shows no diff",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				getParameterHistoryFunc: func(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
					return &paramapi.GetParameterHistoryOutput{
						Parameters: []paramapi.ParameterHistory{
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("same-value"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
							{Name: lo.ToPtr("/app/param"), Value: lo.ToPtr("same-value"), Version: 2, LastModifiedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version")
				// No diff should be shown since values are identical
				assert.NotContains(t, output, "-same-value")
				assert.NotContains(t, output, "+same-value")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf, errBuf bytes.Buffer
			r := &log.Runner{
				UseCase: &param.LogUseCase{Client: tt.mock},
				Stdout:  &buf,
				Stderr:  &errBuf,
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
