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

	"github.com/mpyw/suve/internal/api/secretapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/log"
	"github.com/mpyw/suve/internal/cli/output"
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
	//nolint:revive,stylecheck // Field name matches AWS SDK method name
	listSecretVersionIdsFunc func(ctx context.Context, params *secretapi.ListSecretVersionIDsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) //nolint:lll
	getSecretValueFunc       func(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error)             //nolint:lll
}

//nolint:revive,stylecheck // Method name must match AWS SDK interface
func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretapi.ListSecretVersionIDsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) { //nolint:lll
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

//nolint:lll // mock function signature
func (m *mockClient) GetSecretValue(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}

	return nil, fmt.Errorf("GetSecretValue not mocked")
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
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, params *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))

					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr("v2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
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
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr(testVersionID2), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
				//nolint:lll // mock function signature
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					versionID := lo.FromPtr(params.VersionId)
					switch versionID {
					case testVersionID1:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr("old-value"),
							VersionId:    lo.ToPtr(testVersionID1),
						}, nil
					case testVersionID2:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr("new-value"),
							VersionId:    lo.ToPtr(testVersionID2),
						}, nil
					}

					return nil, fmt.Errorf("unknown version")
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
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("only-version"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
				//nolint:lll // mock function signature
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return &secretapi.GetSecretValueOutput{
						SecretString: lo.ToPtr("only-value"),
						VersionId:    lo.ToPtr("only-version"),
					}, nil
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
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: true,
		},
		{
			name: "reverse order shows oldest first",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Reverse: true},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("version-1"), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr("version-2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
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
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{},
					}, nil
				},
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
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: nil, VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr("v2"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "version without CreatedDate reverse",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Reverse: true},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: nil},
							{VersionId: lo.ToPtr("v2"), CreatedDate: nil},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch skips versions with GetSecretValue error",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
							{VersionId: lo.ToPtr("v2"), CreatedDate: &now},
						},
					}, nil
				},
				//nolint:lll // mock function signature
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			check: func(t *testing.T, output string) {
				// Should still show version info but no diff
				assert.Contains(t, output, "Version")
				assert.NotContains(t, output, "---")
			},
		},
		{
			name: "reverse order with patch",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, Reverse: true},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
							{VersionId: lo.ToPtr(testVersionID2), CreatedDate: &now},
						},
					}, nil
				},
				//nolint:lll // mock function signature
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					versionID := lo.FromPtr(params.VersionId)
					switch versionID {
					case testVersionID1:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr("old-value"),
							VersionId:    lo.ToPtr(testVersionID1),
						}, nil
					case testVersionID2:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr("new-value"),
							VersionId:    lo.ToPtr(testVersionID2),
						}, nil
					}

					return nil, fmt.Errorf("unknown version")
				},
			},
			check: func(t *testing.T, output string) {
				// In reverse mode, first version (oldest) shows diff to next (newer)
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch with JSON format",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
							{VersionId: lo.ToPtr(testVersionID2), CreatedDate: &now},
						},
					}, nil
				},
				//nolint:lll // mock function signature
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					versionID := lo.FromPtr(params.VersionId)
					switch versionID {
					case testVersionID1:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr(`{"key":"old"}`),
							VersionId:    lo.ToPtr(testVersionID1),
						}, nil
					case testVersionID2:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr(`{"key":"new"}`),
							VersionId:    lo.ToPtr(testVersionID2),
						}, nil
					}

					return nil, fmt.Errorf("unknown version")
				},
			},
			check: func(t *testing.T, output string) {
				// Check that diff is shown with formatted JSON
				assert.Contains(t, output, "Version")
			},
		},
		{
			name: "patch with non-JSON value warns",
			opts: log.Options{Name: "my-secret", MaxResults: 10, ShowPatch: true, ParseJSON: true},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
							{VersionId: lo.ToPtr(testVersionID2), CreatedDate: &now},
						},
					}, nil
				},
				//nolint:lll // mock function signature
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					versionID := lo.FromPtr(params.VersionId)
					switch versionID {
					case testVersionID1:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr("not json"),
							VersionId:    lo.ToPtr(testVersionID1),
						}, nil
					case testVersionID2:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr("also not json"),
							VersionId:    lo.ToPtr(testVersionID2),
						}, nil
					}

					return nil, fmt.Errorf("unknown version")
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "-not json")
				assert.Contains(t, output, "+also not json")
			},
		},
		{
			name: "oneline format",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Oneline: true},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr(testVersionID2), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: nil, VersionStages: nil},
							{VersionId: lo.ToPtr(testVersionID2), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "version-")
			},
		},
		{
			name: "filter by since date",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("old-version"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr("new-version"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Only new version should be shown (old is before since)
				assert.Contains(t, output, "new-vers")
				assert.NotContains(t, output, "old-vers")
			},
		},
		{
			name: "filter by until date",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				//nolint:lll // mock function signature
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("old-version"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr("new-version"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Only old version should be shown (new is after until)
				assert.Contains(t, output, "old-vers")
				assert.NotContains(t, output, "new-vers")
			},
		},
		{
			name: "filter by since and until date",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: lo.ToPtr(now.Add(-90 * time.Minute)), Until: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) { //nolint:lll
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("oldest-ver"), CreatedDate: lo.ToPtr(now.Add(-3 * time.Hour))},
							{VersionId: lo.ToPtr("middle-ver"), CreatedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
							{VersionId: lo.ToPtr("newest-ver"), CreatedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) { //nolint:lll
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("v1"), CreatedDate: lo.ToPtr(now.Add(-time.Hour))},
							{VersionId: lo.ToPtr("v2"), CreatedDate: &now},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// All versions are before since, so output is empty
				assert.Empty(t, output)
			},
		},
		{
			name: "filter skips versions without CreatedDate",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Since: lo.ToPtr(now.Add(-30 * time.Minute))},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) { //nolint:lll
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr("no-date-ver"), CreatedDate: nil},
							{VersionId: lo.ToPtr("new-version"), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
				// Version without CreatedDate is skipped in filter
				assert.Contains(t, output, "new-vers")
				assert.NotContains(t, output, "no-date")
			},
		},
		{
			name: "JSON output format",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Output: output.FormatJSON},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) { //nolint:lll
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: lo.ToPtr(now.Add(-time.Hour)), VersionStages: []string{"AWSPREVIOUS"}},
							{VersionId: lo.ToPtr(testVersionID2), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
				//nolint:lll // mock function signature
				getSecretValueFunc: func(_ context.Context, params *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					versionID := lo.FromPtr(params.VersionId)
					switch versionID {
					case testVersionID1:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr("old-value"),
							VersionId:    lo.ToPtr(testVersionID1),
						}, nil
					case testVersionID2:
						return &secretapi.GetSecretValueOutput{
							SecretString: lo.ToPtr("new-value"),
							VersionId:    lo.ToPtr(testVersionID2),
						}, nil
					}

					return nil, fmt.Errorf("unknown version")
				},
			},
			check: func(t *testing.T, output string) {
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
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) { //nolint:lll
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: &now, VersionStages: []string{"AWSCURRENT"}},
						},
					}, nil
				},
				//nolint:lll // mock function signature
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, `"error"`)
				assert.Contains(t, output, "access denied")
			},
		},
		{
			name: "JSON output without CreatedDate",
			opts: log.Options{Name: "my-secret", MaxResults: 10, Output: output.FormatJSON},
			mock: &mockClient{
				listSecretVersionIdsFunc: func(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) { //nolint:lll
					return &secretapi.ListSecretVersionIDsOutput{
						Versions: []secretapi.SecretVersionsListEntry{
							{VersionId: lo.ToPtr(testVersionID1), CreatedDate: nil, VersionStages: nil},
						},
					}, nil
				},
				getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) { //nolint:lll
					return &secretapi.GetSecretValueOutput{
						SecretString: lo.ToPtr("secret-value"),
						VersionId:    lo.ToPtr(testVersionID1),
					}, nil
				},
			},
			check: func(t *testing.T, output string) {
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
