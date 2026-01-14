package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// writeV3State writes a V3 format state file.
//
//nolint:wsl_v5 // test helper with compact JSON building
func writeV3State(t *testing.T, path string, service staging.Service, entries map[string]staging.Entry, tags map[string]staging.TagEntry) {
	t.Helper()

	// Build V3 JSON manually
	var data bytes.Buffer
	data.WriteString(`{"version":3,"service":"`)
	data.WriteString(string(service))
	data.WriteString(`"`)

	if len(entries) > 0 {
		data.WriteString(`,"entries":{`)
		first := true
		for name, entry := range entries {
			if !first {
				data.WriteString(",")
			}
			first = false

			data.WriteString(`"` + name + `":{"operation":"` + string(entry.Operation) + `"`)
			if entry.Value != nil {
				data.WriteString(`,"value":"` + *entry.Value + `"`)
			}
			data.WriteString(`,"staged_at":"` + entry.StagedAt.Format(time.RFC3339Nano) + `"`)
			data.WriteString(`}`)
		}
		data.WriteString(`}`)
	}

	if len(tags) > 0 {
		data.WriteString(`,"tags":{`)
		first := true
		for name, tagEntry := range tags {
			if !first {
				data.WriteString(",")
			}
			first = false

			data.WriteString(`"` + name + `":{`)
			if len(tagEntry.Add) > 0 {
				data.WriteString(`"add":{`)
				addFirst := true
				for k, v := range tagEntry.Add {
					if !addFirst {
						data.WriteString(",")
					}
					addFirst = false
					data.WriteString(`"` + k + `":"` + v + `"`)
				}
				data.WriteString(`}`)
			}
			if len(tagEntry.Remove) > 0 {
				if len(tagEntry.Add) > 0 {
					data.WriteString(",")
				}
				data.WriteString(`"remove":[`)
				removeFirst := true
				for k := range tagEntry.Remove {
					if !removeFirst {
						data.WriteString(",")
					}
					removeFirst = false
					data.WriteString(`"` + k + `"`)
				}
				data.WriteString(`]`)
			}
			data.WriteString(`}`)
		}
		data.WriteString(`}`)
	}

	data.WriteString(`}`)

	require.NoError(t, os.WriteFile(path, data.Bytes(), 0o600))
}

//nolint:funlen // Table-driven test with many cases
func TestStashShowRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("success - show all services with composite store", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")
		secretPath := filepath.Join(tmpDir, "secret.json")

		// Write V3 format test data for both services
		writeV3State(t, paramPath, staging.ServiceParam, map[string]staging.Entry{
			"/app/config": {Operation: staging.OperationUpdate, Value: lo.ToPtr("test-value"), StagedAt: time.Now()},
		}, nil)
		writeV3State(t, secretPath, staging.ServiceSecret, map[string]staging.Entry{
			"my-secret": {Operation: staging.OperationCreate, Value: lo.ToPtr("secret-value"), StagedAt: time.Now()},
		}, nil)

		stores := map[staging.Service]*file.Store{
			staging.ServiceParam:  file.NewStoreWithPath(paramPath, staging.ServiceParam),
			staging.ServiceSecret: file.NewStoreWithPath(secretPath, staging.ServiceSecret),
		}
		fileStore := file.NewCompositeStore(stores)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context(), cli.StashShowOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "my-secret")
		assert.Contains(t, stdout.String(), "Total: 2 stashed item(s)")
	})

	t.Run("success - show specific service", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")

		// Write V3 format test data
		writeV3State(t, paramPath, staging.ServiceParam, map[string]staging.Entry{
			"/app/config": {Operation: staging.OperationUpdate, Value: lo.ToPtr("test-value"), StagedAt: time.Now()},
		}, nil)

		fileStore := file.NewStoreWithPath(paramPath, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context(), cli.StashShowOptions{Service: staging.ServiceParam})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "Total: 1 stashed item(s)")
	})

	t.Run("success - show with tags (add and remove)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")

		// Write V3 format test data with tags
		writeV3State(t, paramPath, staging.ServiceParam, nil, map[string]staging.TagEntry{
			"/app/config": {Add: map[string]string{"env": "prod"}, Remove: maputil.NewSet("old-tag")},
		})

		fileStore := file.NewStoreWithPath(paramPath, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context(), cli.StashShowOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "+1 tags")
		assert.Contains(t, stdout.String(), "-1 tags")
	})

	t.Run("success - show with tags (add only)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")

		// Write V3 format test data with tags (add only)
		writeV3State(t, paramPath, staging.ServiceParam, nil, map[string]staging.TagEntry{
			"/app/config": {Add: map[string]string{"env": "prod", "team": "backend"}, Remove: maputil.NewSet[string]()},
		})

		fileStore := file.NewStoreWithPath(paramPath, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context(), cli.StashShowOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "+2 tags")
		assert.NotContains(t, stdout.String(), "-")
	})

	t.Run("success - show with tags (remove only)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")

		// Write V3 format test data with tags (remove only)
		writeV3State(t, paramPath, staging.ServiceParam, nil, map[string]staging.TagEntry{
			"/app/config": {Add: map[string]string{}, Remove: maputil.NewSet("deprecated", "obsolete")},
		})

		fileStore := file.NewStoreWithPath(paramPath, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context(), cli.StashShowOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "-2 tags")
		assert.NotContains(t, stdout.String(), "+")
	})

	t.Run("error - no stashed changes", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")
		// Don't create the file

		fileStore := file.NewStoreWithPath(paramPath, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context(), cli.StashShowOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no stashed changes")
	})

	t.Run("error - no stashed changes for specific service", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")
		secretPath := filepath.Join(tmpDir, "secret.json")

		// Write V3 format test data with only param service
		writeV3State(t, paramPath, staging.ServiceParam, map[string]staging.Entry{
			"/app/config": {Operation: staging.OperationUpdate, Value: lo.ToPtr("test-value"), StagedAt: time.Now()},
		}, nil)

		stores := map[staging.Service]*file.Store{
			staging.ServiceParam:  file.NewStoreWithPath(paramPath, staging.ServiceParam),
			staging.ServiceSecret: file.NewStoreWithPath(secretPath, staging.ServiceSecret),
		}
		fileStore := file.NewCompositeStore(stores)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		// Try to show secret service which has no entries
		err := runner.Run(t.Context(), cli.StashShowOptions{Service: staging.ServiceSecret})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no stashed changes for secret")
	})

	t.Run("success - verbose output", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")

		// Write V3 format test data
		writeV3State(t, paramPath, staging.ServiceParam, map[string]staging.Entry{
			"/app/config": {Operation: staging.OperationUpdate, Value: lo.ToPtr("test-value"), StagedAt: time.Now()},
		}, nil)

		fileStore := file.NewStoreWithPath(paramPath, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context(), cli.StashShowOptions{Verbose: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		// Verbose output includes the value
		assert.Contains(t, stdout.String(), "test-value")
	})

	t.Run("file preserved after show", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")

		// Write V3 format test data
		writeV3State(t, paramPath, staging.ServiceParam, map[string]staging.Entry{
			"/app/config": {Operation: staging.OperationUpdate, Value: lo.ToPtr("test-value"), StagedAt: time.Now()},
		}, nil)

		fileStore := file.NewStoreWithPath(paramPath, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context(), cli.StashShowOptions{})
		require.NoError(t, err)

		// File should still exist
		_, err = os.Stat(paramPath)
		assert.NoError(t, err)
	})
}
