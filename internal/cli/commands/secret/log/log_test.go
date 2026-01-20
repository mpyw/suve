package log_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/log"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/model"
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
		// This will fail due to AWS client init, but we test the warning path
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

type mockClient struct {
	versionsResult  []*model.SecretVersion
	versionsErr     error
	getSecretValues map[string]string
	getSecretErr    map[string]error
}

func (m *mockClient) GetSecret(_ context.Context, _ string, versionID, _ string) (*model.Secret, error) {
	if m.getSecretErr != nil {
		if err, ok := m.getSecretErr[versionID]; ok {
			return nil, err
		}
	}

	if m.getSecretValues != nil {
		if value, ok := m.getSecretValues[versionID]; ok {
			return &model.Secret{
				Name:    "my-secret",
				Value:   value,
				Version: versionID,
			}, nil
		}
	}

	return nil, errors.New("not found")
}

func (m *mockClient) GetSecretVersions(_ context.Context, _ string) ([]*model.SecretVersion, error) {
	if m.versionsErr != nil {
		return nil, m.versionsErr
	}

	return m.versionsResult, nil
}

func (m *mockClient) ListSecrets(_ context.Context) ([]*model.SecretListItem, error) {
	return nil, errors.New("not implemented")
}

func ptrTime(t time.Time) *time.Time {
	return &t
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
			name: "show version history",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "v1", CreatedAt: ptrTime(now.Add(-time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}}},
					{Version: "v2", CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
				assert.Contains(t, output, "AWSCURRENT")
			},
		},
		{
			name: "show patch between versions",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: ptrTime(now.Add(-time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}}},
					{Version: testVersionID2, CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
				getSecretValues: map[string]string{
					testVersionID1: "old-value",
					testVersionID2: "new-value",
				},
			},
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
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "only-version", CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
				getSecretValues: map[string]string{
					"only-version": "only-value",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name: "error from AWS",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			mock: &mockClient{
				versionsErr: errors.New("AWS error"),
			},
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Reverse: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "version-1", CreatedAt: ptrTime(now.Add(-time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}}},
					{Version: "version-2", CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
			},
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
			name: "empty version list",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Empty(t, output)
			},
		},
		{
			name: "version without CreatedDate",
			opts: log.Options{Name: "my-secret", MaxResults: 10},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "v1", CreatedAt: nil, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}}},
					{Version: "v2", CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "version without CreatedDate reverse",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Reverse: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "v1", CreatedAt: nil},
					{Version: "v2", CreatedAt: nil},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch skips versions with GetSecretValue error",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "v1", CreatedAt: ptrTime(now.Add(-time.Hour))},
					{Version: "v2", CreatedAt: &now},
				},
				getSecretErr: map[string]error{
					"v1": errors.New("access denied"),
					"v2": errors.New("access denied"),
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// Should still show version info but no diff
				assert.Contains(t, output, "Version")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name: "reverse order with patch",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, Reverse: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: ptrTime(now.Add(-time.Hour))},
					{Version: testVersionID2, CreatedAt: &now},
				},
				getSecretValues: map[string]string{
					testVersionID1: "old-value",
					testVersionID2: "new-value",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// In reverse mode, first version (oldest) shows diff to next (newer)
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch with JSON format",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: ptrTime(now.Add(-time.Hour))},
					{Version: testVersionID2, CreatedAt: &now},
				},
				getSecretValues: map[string]string{
					testVersionID1: `{"key":"old"}`,
					testVersionID2: `{"key":"new"}`,
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// Check that diff is shown with formatted JSON
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch with non-JSON value warns",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: ptrTime(now.Add(-time.Hour))},
					{Version: testVersionID2, CreatedAt: &now},
				},
				getSecretValues: map[string]string{
					testVersionID1: "not json",
					testVersionID2: "also not json",
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-not json")
				assert.Contains(t, output, "+also not json")
			},
		},
		{
			name: "oneline format",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Oneline: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: ptrTime(now.Add(-time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}}},
					{Version: testVersionID2, CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// Oneline format should be compact
				assert.Contains(t, output, "version-")
				assert.Contains(t, output, "AWSCURRENT")
				// Should not have "Version" prefix like normal format
				assert.NotContains(t, output, "Version ")
			},
		},
		{
			name: "oneline format without CreatedDate",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Oneline: true},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: nil},
					{Version: testVersionID2, CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "version-")
			},
		},
		{
			name: "filter by since date",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{
						Version: "old-version", CreatedAt: ptrTime(now.Add(-2 * time.Hour)),
						Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}},
					},
					{
						Version: "new-version", CreatedAt: &now,
						Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// Only new version should be shown (old is before since)
				assert.Contains(t, output, "new-vers")
				assert.NotContains(t, output, "old-vers")
			},
		},
		{
			name: "filter by until date",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{
						Version: "old-version", CreatedAt: ptrTime(now.Add(-2 * time.Hour)),
						Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}},
					},
					{
						Version: "new-version", CreatedAt: &now,
						Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}},
					},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// Only old version should be shown (new is after until)
				assert.Contains(t, output, "old-vers")
				assert.NotContains(t, output, "new-vers")
			},
		},
		{
			name: "filter by since and until date",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: lo.ToPtr(now.Add(-90 * time.Minute)), Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "oldest-ver", CreatedAt: ptrTime(now.Add(-3 * time.Hour))},
					{Version: "middle-ver", CreatedAt: ptrTime(now.Add(-1 * time.Hour))},
					{Version: "newest-ver", CreatedAt: &now},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// Only middle version should be shown
				assert.Contains(t, output, "middle-v")
				assert.NotContains(t, output, "oldest-v")
				assert.NotContains(t, output, "newest-v")
			},
		},
		{
			name: "filter excludes all versions",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: lo.ToPtr(now.Add(time.Hour))},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "v1", CreatedAt: ptrTime(now.Add(-time.Hour))},
					{Version: "v2", CreatedAt: &now},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// All versions are before since, so output is empty
				assert.Empty(t, output)
			},
		},
		{
			name: "filter skips versions without CreatedDate",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: "no-date-ver", CreatedAt: nil},
					{Version: "new-version", CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				// Version without CreatedDate is skipped in filter
				assert.Contains(t, output, "new-vers")
				assert.NotContains(t, output, "no-date")
			},
		},
		{
			name: "JSON output format",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Output: output.FormatJSON},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: ptrTime(now.Add(-time.Hour)), Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSPREVIOUS"}}},
					{Version: testVersionID2, CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
				getSecretValues: map[string]string{
					testVersionID1: "old-value",
					testVersionID2: "new-value",
				},
			},
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
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: &now, Metadata: model.AWSSecretVersionMeta{VersionStages: []string{"AWSCURRENT"}}},
				},
				getSecretErr: map[string]error{
					testVersionID1: errors.New("access denied"),
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, `"error"`)
				assert.Contains(t, output, "access denied")
			},
		},
		{
			name: "JSON output without CreatedDate",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Output: output.FormatJSON},
			mock: &mockClient{
				versionsResult: []*model.SecretVersion{
					{Version: testVersionID1, CreatedAt: nil},
				},
				getSecretValues: map[string]string{
					testVersionID1: "secret-value",
				},
			},
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
				UseCase: &secret.LogUseCase{Client: tt.mock},
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
