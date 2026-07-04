package log_test

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/log"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
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

// logVer describes one version for the log-store helper (oldest-first input).
type logVer struct {
	ver      int64
	value    string
	typ      domain.ValueType
	modified *time.Time
}

// logStore builds a provider mock: History returns the versions newest-first,
// and Resolve/Get fetch a version's value/type by id (mirroring the adapter).
func logStore(oldestFirst []logVer) *providermock.Store {
	byID := make(map[string]logVer, len(oldestFirst))

	versionsNewestFirst := make([]domain.Version, 0, len(oldestFirst))

	for i := len(oldestFirst) - 1; i >= 0; i-- {
		v := oldestFirst[i]
		id := strconv.FormatInt(v.ver, 10)
		byID[id] = v
		versionsNewestFirst = append(versionsNewestFirst, domain.Version{ID: id, Created: v.modified})
	}

	return &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return versionsNewestFirst, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			v := byID[ref.ID()]

			return &domain.Entry{
				Name:     name,
				Value:    v.value,
				Type:     v.typ,
				Version:  domain.Version{ID: ref.ID(), Created: v.modified},
				Modified: v.modified,
			}, nil
		},
	}
}

//nolint:funlen // Table-driven test with many cases
func TestRun(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		opts    log.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show history",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 2, value: "v2", modified: &now},
			}),
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
			store: logStore([]logVer{
				{ver: 1, value: "this is a very long value that should NOT be truncated in normal mode", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "should NOT be truncated in normal mode")
				assert.NotContains(t, output, "...")
			},
		},
		{
			name: "max-value-length truncates in normal mode",
			opts: log.Options{Name: "/app/param", MaxResults: 10, MaxValueLength: 20},
			store: logStore([]logVer{
				{ver: 1, value: "this is a very long value that should be truncated", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "...")
				assert.NotContains(t, output, "should be truncated")
			},
		},
		{
			name: "show patch between versions",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true},
			store: logStore([]logVer{
				{ver: 1, value: "old-value", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 2, value: "new-value", modified: &now},
			}),
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
			store: logStore([]logVer{
				{ver: 1, value: "only-value", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 1")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name: "error from AWS",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			store: &providermock.Store{
				HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
					return nil, assert.AnError
				},
			},
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Reverse: true},
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 2, value: "v2", modified: &now},
			}),
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
			store: logStore([]logVer{
				{ver: 1, value: "old-value", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 2, value: "new-value", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-old-value")
				assert.Contains(t, output, "+new-value")
				assert.Contains(t, output, "--- /app/param#1")
				assert.Contains(t, output, "+++ /app/param#2")
			},
		},
		{
			name:  "empty history",
			opts:  log.Options{Name: "/app/param", MaxResults: 10},
			store: logStore(nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "oneline format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Oneline: true},
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 2, value: "v2", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "2")
				assert.Contains(t, output, "(current)")
				assert.Contains(t, output, "v2")
				assert.NotContains(t, output, "Version 2")
			},
		},
		{
			name: "oneline truncates long values",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Oneline: true},
			store: logStore([]logVer{
				{ver: 1, value: "this is a very long value that exceeds forty characters", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "...")
				assert.NotContains(t, output, "exceeds forty characters")
			},
		},
		{
			name: "filter by since date",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-90 * time.Minute))},
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-2 * time.Hour))},
				{ver: 2, value: "v2", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 3, value: "v3", modified: &now},
			}),
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
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-2 * time.Hour))},
				{ver: 2, value: "v2", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 3, value: "v3", modified: &now},
			}),
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
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-3 * time.Hour))},
				{ver: 2, value: "v2", modified: lo.ToPtr(now.Add(-2 * time.Hour))},
				{ver: 3, value: "v3", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 4, value: "v4", modified: &now},
			}),
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
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "filter skips versions without LastModifiedDate",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-30 * time.Minute))},
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: nil},
				{ver: 2, value: "v2", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 2")
				assert.NotContains(t, output, "Version 1")
			},
		},
		{
			name: "JSON output format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Output: output.FormatJSON},
			store: logStore([]logVer{
				{ver: 1, value: "value1", typ: domain.ValueTypePlaintext, modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 2, value: "value2", typ: domain.ValueTypeSecret, modified: &now},
			}),
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
			store: logStore([]logVer{
				{ver: 1, value: "value1", typ: domain.ValueTypePlaintext, modified: nil},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"version"`)
				assert.NotContains(t, output, `"modified"`)
			},
		},
		{
			name: "patch with JSON format",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			store: logStore([]logVer{
				{ver: 1, value: `{"key":"old"}`, modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 2, value: `{"key":"new"}`, modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "version without LastModifiedDate shows correctly",
			opts: log.Options{Name: "/app/param", MaxResults: 10},
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: nil},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version 1")
				assert.NotContains(t, output, "Date:")
			},
		},
		{
			name: "oneline without LastModifiedDate",
			opts: log.Options{Name: "/app/param", MaxResults: 10, Oneline: true},
			store: logStore([]logVer{
				{ver: 1, value: "v1", modified: nil},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "1")
			},
		},
		{
			name: "patch with identical values shows no diff",
			opts: log.Options{Name: "/app/param", MaxResults: 10, ShowPatch: true},
			store: logStore([]logVer{
				{ver: 1, value: "same-value", modified: lo.ToPtr(now.Add(-time.Hour))},
				{ver: 2, value: "same-value", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
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
				UseCase: &param.LogUseCase{Reader: tt.store},
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
