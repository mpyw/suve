package log_test

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	cmdparam "github.com/mpyw/suve/internal/cli/commands/param"
	cmdsecret "github.com/mpyw/suve/internal/cli/commands/secret"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("errors", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name    string
			args    []string
			wantSub string
		}{
			{"param missing name", []string{"suve", "param", "log"}, "usage: suve param log"},
			{"param invalid since", []string{"suve", "param", "log", "--since", "invalid", "/app/param"}, "invalid --since value"},
			{"param invalid until", []string{"suve", "param", "log", "--until", "invalid", "/app/param"}, "invalid --until value"},
			{"secret missing name", []string{"suve", "secret", "log"}, "usage: suve secret log"},
			{"secret invalid since", []string{"suve", "secret", "log", "--since", "invalid", "my-secret"}, "invalid --since value"},
			{"secret invalid until", []string{"suve", "secret", "log", "--until", "not-a-date", "my-secret"}, "invalid --until value"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				app := appcli.MakeApp()
				err := app.Run(t.Context(), tc.args)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantSub)
			})
		}
	})

	t.Run("warnings", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name    string
			args    []string
			wantSub string
		}{
			{"param json without patch", []string{"suve", "param", "log", "--parse-json", "/app/param"}, "--parse-json has no effect"},
			{"param oneline with patch", []string{"suve", "param", "log", "--oneline", "--patch", "/app/param"}, "--oneline has no effect"},
			{"param json with patch", []string{"suve", "param", "log", "--output=json", "--patch", "/app/param"}, "-p/--patch has no effect"},
			{"param json with oneline", []string{"suve", "param", "log", "--output=json", "--oneline", "/app/param"}, "--oneline has no effect"},
			{"secret json without patch", []string{"suve", "secret", "log", "--parse-json", "my-secret"}, "--parse-json has no effect"},
			{"secret oneline with patch", []string{"suve", "secret", "log", "--oneline", "--patch", "my-secret"}, "--oneline has no effect"},
			{"secret json with patch", []string{"suve", "secret", "log", "--output=json", "--patch", "my-secret"}, "-p/--patch has no effect"},
			{"secret json with oneline", []string{"suve", "secret", "log", "--output=json", "--oneline", "my-secret"}, "--oneline has no effect"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var errBuf bytes.Buffer

				app := appcli.MakeApp()
				app.ErrWriter = &errBuf
				_ = app.Run(t.Context(), tc.args)
				assert.Contains(t, errBuf.String(), tc.wantSub)
			})
		}
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
}

// === param log ===

// paramLogVer describes one version for the param log-store helper (oldest-first input).
type paramLogVer struct {
	ver      int64
	value    string
	typ      domain.ValueType
	modified *time.Time
}

// paramLogStore builds a provider mock: History returns the versions newest-first,
// and Resolve/Get fetch a version's value/type by id (mirroring the adapter).
func paramLogStore(oldestFirst []paramLogVer) *providermock.Store {
	byID := make(map[string]paramLogVer, len(oldestFirst))

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

func runParam(
	t *testing.T, store *providermock.Store, req genericlog.Request, opts genericlog.Options,
) (string, error) {
	t.Helper()

	presenter := cmdparam.NewLogPresenter(store, req)

	var buf, errBuf bytes.Buffer

	r := &genericlog.Runner{Presenter: presenter, Options: opts, Stdout: &buf, Stderr: &errBuf}
	err := r.Run(t.Context())

	return buf.String(), err
}

//nolint:funlen // Table-driven test with many cases
func TestRunParam(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		req     genericlog.Request
		opts    genericlog.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show history",
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{MaxValueLength: 20},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			store: &providermock.Store{
				HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
					return nil, assert.AnError
				},
			},
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10, Reverse: true},
			opts: genericlog.Options{Reverse: true},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10, Reverse: true},
			opts: genericlog.Options{ShowPatch: true, Reverse: true},
			store: paramLogStore([]paramLogVer{
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
			req:   genericlog.Request{Name: "/app/param", MaxResults: 10},
			store: paramLogStore(nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "oneline format",
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{Oneline: true},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{Oneline: true},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-90 * time.Minute))},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10, Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			store: paramLogStore([]paramLogVer{
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
			req: genericlog.Request{
				Name: "/app/param", MaxResults: 10,
				Since: lo.ToPtr(now.Add(-150 * time.Minute)), Until: lo.ToPtr(now.Add(-30 * time.Minute)),
			},
			store: paramLogStore([]paramLogVer{
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
			req: genericlog.Request{
				Name: "/app/param", MaxResults: 10,
				Since: lo.ToPtr(now.Add(time.Hour)), Until: lo.ToPtr(now.Add(2 * time.Hour)),
			},
			store: paramLogStore([]paramLogVer{
				{ver: 1, value: "v1", modified: &now},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "filter skips versions without LastModifiedDate",
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10, Since: lo.ToPtr(now.Add(-30 * time.Minute))},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{Output: output.FormatJSON},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{Output: output.FormatJSON},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true, ParseJSON: true},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			store: paramLogStore([]paramLogVer{
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
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{Oneline: true},
			store: paramLogStore([]paramLogVer{
				{ver: 1, value: "v1", modified: nil},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "1")
			},
		},
		{
			name: "patch with identical values shows no diff",
			req:  genericlog.Request{Name: "/app/param", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true},
			store: paramLogStore([]paramLogVer{
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

			out, err := runParam(t, tt.store, tt.req, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// === secret log ===

const (
	testVersionID1 = "version-id-1"
	testVersionID2 = "version-id-2"
)

// secretLogStore builds a mock reader whose History returns the given versions
// (newest first) and whose Get returns per-version-id values/errors.
func secretLogStore(
	versions []domain.Version, values map[string]string, errs map[string]error, historyErr error,
) *providermock.Store {
	return &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return versions, historyErr
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			id := ref.ID()
			if errs != nil {
				if err, ok := errs[id]; ok {
					return nil, err
				}
			}

			return &domain.Entry{Value: values[id], Version: domain.Version{ID: id}}, nil
		},
	}
}

func at(now time.Time, d time.Duration) *time.Time {
	tt := now.Add(d)

	return &tt
}

func runSecret(
	t *testing.T, store *providermock.Store, req genericlog.Request, opts genericlog.Options,
) (string, error) {
	t.Helper()

	presenter := cmdsecret.NewLogPresenter(store, req)

	var buf, errBuf bytes.Buffer

	r := &genericlog.Runner{Presenter: presenter, Options: opts, Stdout: &buf, Stderr: &errBuf}
	err := r.Run(t.Context())

	return buf.String(), err
}

//nolint:funlen // Table-driven test with many cases
func TestRunSecret(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		req     genericlog.Request
		opts    genericlog.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "show version history",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			store: secretLogStore([]domain.Version{
				{ID: "v2", Label: "AWSCURRENT", Created: &now},
				{ID: "v1", Label: "AWSPREVIOUS", Created: at(now, -time.Hour)},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
				assert.Contains(t, output, "AWSCURRENT")
			},
		},
		{
			name: "show patch between versions",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID2, Label: "AWSCURRENT", Created: &now},
				{ID: testVersionID1, Label: "AWSPREVIOUS", Created: at(now, -time.Hour)},
			}, map[string]string{testVersionID1: "old-value", testVersionID2: "new-value"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-old-value")
				assert.Contains(t, output, "+new-value")
				assert.Contains(t, output, "my-secret#version-")
			},
		},
		{
			name: "patch with single version shows no diff",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true},
			store: secretLogStore([]domain.Version{
				{ID: "only-version", Label: "AWSCURRENT", Created: &now},
			}, map[string]string{"only-version": "only-value"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name:    "error from AWS",
			req:     genericlog.Request{Name: "my-secret", MaxResults: 10},
			store:   secretLogStore(nil, nil, nil, errors.New("AWS error")),
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10, Reverse: true},
			opts: genericlog.Options{Reverse: true},
			store: secretLogStore([]domain.Version{
				{ID: "version-2", Label: "AWSCURRENT", Created: &now},
				{ID: "version-1", Label: "AWSPREVIOUS", Created: at(now, -time.Hour)},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()

				prevPos := strings.Index(output, "AWSPREVIOUS")
				currPos := strings.Index(output, "AWSCURRENT")

				require.NotEqual(t, -1, prevPos, "expected AWSPREVIOUS in output")
				require.NotEqual(t, -1, currPos, "expected AWSCURRENT in output")
				assert.Less(t, prevPos, currPos, "expected AWSPREVIOUS before AWSCURRENT in reverse mode")
			},
		},
		{
			name:  "empty version list",
			req:   genericlog.Request{Name: "my-secret", MaxResults: 10},
			store: secretLogStore(nil, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "version without CreatedDate",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			store: secretLogStore([]domain.Version{
				{ID: "v2", Label: "AWSCURRENT", Created: &now},
				{ID: "v1", Label: "AWSPREVIOUS"},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "version without CreatedDate reverse",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10, Reverse: true},
			opts: genericlog.Options{Reverse: true},
			store: secretLogStore([]domain.Version{
				{ID: "v1"},
				{ID: "v2"},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch skips versions with GetSecretValue error",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true},
			store: secretLogStore([]domain.Version{
				{ID: "v2", Created: &now},
				{ID: "v1", Created: at(now, -time.Hour)},
			}, nil, map[string]error{"v1": errors.New("access denied"), "v2": errors.New("access denied")}, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name: "reverse order with patch",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10, Reverse: true},
			opts: genericlog.Options{ShowPatch: true, Reverse: true},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID2, Created: &now},
				{ID: testVersionID1, Created: at(now, -time.Hour)},
			}, map[string]string{testVersionID1: "old-value", testVersionID2: "new-value"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch with JSON format",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true, ParseJSON: true},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID2, Created: &now},
				{ID: testVersionID1, Created: at(now, -time.Hour)},
			}, map[string]string{testVersionID1: `{"key":"old"}`, testVersionID2: `{"key":"new"}`}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch with non-JSON value warns",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{ShowPatch: true, ParseJSON: true},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID2, Created: &now},
				{ID: testVersionID1, Created: at(now, -time.Hour)},
			}, map[string]string{testVersionID1: "not json", testVersionID2: "also not json"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-not json")
				assert.Contains(t, output, "+also not json")
			},
		},
		{
			name: "oneline format",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{Oneline: true},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID2, Label: "AWSCURRENT", Created: &now},
				{ID: testVersionID1, Label: "AWSPREVIOUS", Created: at(now, -time.Hour)},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "version-")
				assert.Contains(t, output, "AWSCURRENT")
				assert.NotContains(t, output, "Version ")
			},
		},
		{
			name: "oneline format without CreatedDate",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{Oneline: true},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID2, Label: "AWSCURRENT", Created: &now},
				{ID: testVersionID1},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "version-")
			},
		},
		{
			name: "filter by since date",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10, Since: at(now, -30*time.Minute)},
			store: secretLogStore([]domain.Version{
				{ID: "new-version", Label: "AWSCURRENT", Created: &now},
				{ID: "old-version", Label: "AWSPREVIOUS", Created: at(now, -2*time.Hour)},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "new-vers")
				assert.NotContains(t, output, "old-vers")
			},
		},
		{
			name: "filter by until date",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10, Until: at(now, -30*time.Minute)},
			store: secretLogStore([]domain.Version{
				{ID: "new-version", Label: "AWSCURRENT", Created: &now},
				{ID: "old-version", Label: "AWSPREVIOUS", Created: at(now, -2*time.Hour)},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "old-vers")
				assert.NotContains(t, output, "new-vers")
			},
		},
		{
			name: "filter by since and until date",
			req: genericlog.Request{
				Name: "my-secret", MaxResults: 10,
				Since: at(now, -90*time.Minute), Until: at(now, -30*time.Minute),
			},
			store: secretLogStore([]domain.Version{
				{ID: "newest-ver", Created: &now},
				{ID: "middle-ver", Created: at(now, -time.Hour)},
				{ID: "oldest-ver", Created: at(now, -3*time.Hour)},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "middle-v")
				assert.NotContains(t, output, "oldest-v")
				assert.NotContains(t, output, "newest-v")
			},
		},
		{
			name: "filter excludes all versions",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10, Since: at(now, time.Hour)},
			store: secretLogStore([]domain.Version{
				{ID: "v2", Created: &now},
				{ID: "v1", Created: at(now, -time.Hour)},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "filter skips versions without CreatedDate",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10, Since: at(now, -30*time.Minute)},
			store: secretLogStore([]domain.Version{
				{ID: "new-version", Label: "AWSCURRENT", Created: &now},
				{ID: "no-date-ver"},
			}, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "new-vers")
				assert.NotContains(t, output, "no-date")
			},
		},
		{
			name: "JSON output format",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{Output: output.FormatJSON},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID2, Label: "AWSCURRENT", Created: &now},
				{ID: testVersionID1, Label: "AWSPREVIOUS", Created: at(now, -time.Hour)},
			}, map[string]string{testVersionID1: "old-value", testVersionID2: "new-value"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"versionId"`)
				assert.Contains(t, output, `"value"`)
				assert.Contains(t, output, `"stages"`)
				assert.Contains(t, output, "AWSCURRENT")
			},
		},
		{
			name: "JSON output with GetSecretValue error",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{Output: output.FormatJSON},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID1, Label: "AWSCURRENT", Created: &now},
			}, nil, map[string]error{testVersionID1: errors.New("access denied")}, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"error"`)
				assert.Contains(t, output, "access denied")
			},
		},
		{
			name: "JSON output without CreatedDate",
			req:  genericlog.Request{Name: "my-secret", MaxResults: 10},
			opts: genericlog.Options{Output: output.FormatJSON},
			store: secretLogStore([]domain.Version{
				{ID: testVersionID1},
			}, map[string]string{testVersionID1: "secret-value"}, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"versionId"`)
				assert.NotContains(t, output, `"created"`)
				assert.NotContains(t, output, `"stages"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := runSecret(t, tt.store, tt.req, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}
