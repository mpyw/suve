package log_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/log"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

const (
	testVersionID1 = "version-id-1"
	testVersionID2 = "version-id-2"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "log"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve secret log")
	})

	t.Run("json without patch warns", func(t *testing.T) {
		t.Parallel()

		var errBuf bytes.Buffer

		app := appcli.MakeApp()
		app.ErrWriter = &errBuf
		_ = app.Run(t.Context(), []string{"suve", "secret", "log", "--parse-json", "my-secret"})
		assert.Contains(t, errBuf.String(), "--parse-json has no effect")
	})

	t.Run("invalid since timestamp", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "log", "--since", "invalid", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --since value")
	})

	t.Run("invalid until timestamp", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "log", "--until", "not-a-date", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --until value")
	})

	t.Run("oneline with patch warns", func(t *testing.T) {
		t.Parallel()

		var errBuf bytes.Buffer

		app := appcli.MakeApp()
		app.ErrWriter = &errBuf
		_ = app.Run(t.Context(), []string{"suve", "secret", "log", "--oneline", "--patch", "my-secret"})
		assert.Contains(t, errBuf.String(), "--oneline has no effect")
	})

	t.Run("output json with patch warns", func(t *testing.T) {
		t.Parallel()

		var errBuf bytes.Buffer

		app := appcli.MakeApp()
		app.ErrWriter = &errBuf
		_ = app.Run(t.Context(), []string{"suve", "secret", "log", "--output=json", "--patch", "my-secret"})
		assert.Contains(t, errBuf.String(), "-p/--patch has no effect")
	})

	t.Run("output json with oneline warns", func(t *testing.T) {
		t.Parallel()

		var errBuf bytes.Buffer

		app := appcli.MakeApp()
		app.ErrWriter = &errBuf
		_ = app.Run(t.Context(), []string{"suve", "secret", "log", "--output=json", "--oneline", "my-secret"})
		assert.Contains(t, errBuf.String(), "--oneline has no effect")
	})
}

// logStore builds a mock reader whose History returns the given versions
// (newest first) and whose Get returns per-version-id values/errors.
func logStore(
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
			name: "show version history",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true},
			store: logStore([]domain.Version{
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
			opts:    log.Options{Name: "my-secret", MaxResults: 10},
			store:   logStore(nil, nil, nil, errors.New("AWS error")),
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Reverse: true},
			store: logStore([]domain.Version{
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
			opts:  log.Options{Name: "my-secret", MaxResults: 10},
			store: logStore(nil, nil, nil, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "version without CreatedDate",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Reverse: true},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, Reverse: true},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Oneline: true},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Oneline: true},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: at(now, -30*time.Minute)},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Until: at(now, -30*time.Minute)},
			store: logStore([]domain.Version{
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
			opts: log.Options{
				Name: "my-secret", MaxResults: 10,
				Since: at(now, -90*time.Minute), Until: at(now, -30*time.Minute),
			},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: at(now, time.Hour)},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: at(now, -30*time.Minute)},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Output: output.FormatJSON},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Output: output.FormatJSON},
			store: logStore([]domain.Version{
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
			opts: log.Options{Name: "my-secret", MaxResults: 10, Output: output.FormatJSON},
			store: logStore([]domain.Version{
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

			var buf, errBuf bytes.Buffer

			r := &log.Runner{
				UseCase: &secret.LogUseCase{Reader: tt.store},
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
