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

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/log"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/model"
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
	getParameterResult *model.Parameter
	getParameterErr    error
	getHistoryResult   *model.ParameterHistory
	getHistoryErr      error
	listParametersErr  error
}

func (m *mockClient) GetParameter(_ context.Context, _ string, _ string) (*model.Parameter, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}

	return m.getParameterResult, nil
}

func (m *mockClient) GetParameterHistory(_ context.Context, _ string) (*model.ParameterHistory, error) {
	if m.getHistoryErr != nil {
		return nil, m.getHistoryErr
	}

	if m.getHistoryResult == nil {
		return &model.ParameterHistory{}, nil
	}

	return m.getHistoryResult, nil
}

func (m *mockClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	if m.listParametersErr != nil {
		return nil, m.listParametersErr
	}

	return nil, nil
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v2", Version: "2", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{
							Name: "/app/param", Value: "this is a very long value that should NOT be truncated in normal mode",
							Version: "1", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"},
						},
					},
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{
							Name: "/app/param", Value: "this is a very long value that should be truncated",
							Version: "1", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"},
						},
					},
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{
							Name: "/app/param", Value: "old-value", Version: "1",
							UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"},
						},
						{
							Name: "/app/param", Value: "new-value", Version: "2",
							UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"},
						},
					},
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "only-value", Version: "1", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 1")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name:    "error from AWS",
			opts:    log.Options{Name: "/app/param", MaxResults: 10},
			mock:    &mockClient{getHistoryErr: fmt.Errorf("AWS error")},
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Reverse: true},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v2", Version: "2", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{
							Name: "/app/param", Value: "old-value", Version: "1",
							UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"},
						},
						{
							Name: "/app/param", Value: "new-value", Version: "2",
							UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"},
						},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
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
				getHistoryResult: &model.ParameterHistory{
					Name:       "/app/param",
					Parameters: []*model.Parameter{},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "oneline format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Oneline: true},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v2", Version: "2", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{
							Name: "/app/param", Value: "this is a very long value that exceeds forty characters",
							Version: "1", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"},
						},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "...")
				// Full value should not appear
				assert.NotContains(t, output, "exceeds forty characters")
			},
		},
		{
			name: "filter by since date",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-90 * time.Minute))},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: lo.ToPtr(now.Add(-2 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v2", Version: "2", UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v3", Version: "3", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 2")
				assert.Contains(t, output, "Version 3")
				assert.NotContains(t, output, "Version 1")
			},
		},
		{
			name: "filter by until date",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: lo.ToPtr(now.Add(-2 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v2", Version: "2", UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v3", Version: "3", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 1")
				assert.Contains(t, output, "Version 2")
				assert.NotContains(t, output, "Version 3")
			},
		},
		{
			name: "filter by since and until date range",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-150 * time.Minute)), Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: lo.ToPtr(now.Add(-3 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v2", Version: "2", UpdatedAt: lo.ToPtr(now.Add(-2 * time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v3", Version: "3", UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v4", Version: "4", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "filter skips versions without LastModifiedDate",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: nil, Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "v2", Version: "2", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 2")
				assert.NotContains(t, output, "Version 1")
			},
		},
		{
			name: "JSON output format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Output: output.FormatJSON},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "value1", Version: "1", UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"}},
						{Name: "/app/param", Value: "value2", Version: "2", UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "SecureString"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
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
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "value1", Version: "1", UpdatedAt: nil, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"version"`)
				assert.NotContains(t, output, `"modified"`)
			},
		},
		{
			name: "patch with JSON format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{
							Name: "/app/param", Value: `{"key":"old"}`, Version: "1",
							UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"},
						},
						{
							Name: "/app/param", Value: `{"key":"new"}`, Version: "2",
							UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"},
						},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "version without UpdatedAt shows correctly",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: nil, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 1")
				assert.NotContains(t, output, "Date:")
			},
		},
		{
			name: "oneline without LastModifiedDate",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Oneline: true},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{Name: "/app/param", Value: "v1", Version: "1", UpdatedAt: nil, Metadata: model.AWSParameterMeta{Type: "String"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "1")
			},
		},
		{
			name: "patch with identical values shows no diff",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				getHistoryResult: &model.ParameterHistory{
					Name: "/app/param",
					Parameters: []*model.Parameter{
						{
							Name: "/app/param", Value: "same-value", Version: "1",
							UpdatedAt: lo.ToPtr(now.Add(-time.Hour)), Metadata: model.AWSParameterMeta{Type: "String"},
						},
						{
							Name: "/app/param", Value: "same-value", Version: "2",
							UpdatedAt: &now, Metadata: model.AWSParameterMeta{Type: "String"},
						},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()

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
