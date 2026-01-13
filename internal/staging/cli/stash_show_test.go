package cli_test

import (
	"bytes"
	"encoding/json"
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

//nolint:funlen // Table-driven test with many cases
func TestStashShowRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("success - show all services", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		}
		data, err := json.MarshalIndent(state, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))

		fileStore := file.NewStoreWithPath(path)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err = runner.Run(t.Context(), cli.StashShowOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "my-secret")
		assert.Contains(t, stdout.String(), "Total: 2 stashed item(s)")
	})

	t.Run("success - show specific service", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data with both services
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		}
		data, err := json.MarshalIndent(state, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))

		fileStore := file.NewStoreWithPath(path)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err = runner.Run(t.Context(), cli.StashShowOptions{Service: staging.ServiceParam})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.NotContains(t, stdout.String(), "my-secret")
		assert.Contains(t, stdout.String(), "Total: 1 stashed item(s)")
	})

	t.Run("success - show with tags", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data with tags
		state := staging.NewEmptyState()
		state.Tags[staging.ServiceParam]["/app/config"] = staging.TagEntry{
			Add:    map[string]string{"env": "prod"},
			Remove: maputil.NewSet("old-tag"),
		}
		data, err := json.MarshalIndent(state, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))

		fileStore := file.NewStoreWithPath(path)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err = runner.Run(t.Context(), cli.StashShowOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "+1 tags")
		assert.Contains(t, stdout.String(), "-1 tags")
	})

	t.Run("error - no stashed changes", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		// Don't create the file

		fileStore := file.NewStoreWithPath(path)
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
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data with only param service
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		}
		data, err := json.MarshalIndent(state, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))

		fileStore := file.NewStoreWithPath(path)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		// Try to show secret service which has no entries
		err = runner.Run(t.Context(), cli.StashShowOptions{Service: staging.ServiceSecret})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no stashed changes for secret")
	})

	t.Run("success - verbose output", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		}
		data, err := json.MarshalIndent(state, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))

		fileStore := file.NewStoreWithPath(path)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err = runner.Run(t.Context(), cli.StashShowOptions{Verbose: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		// Verbose output includes the value
		assert.Contains(t, stdout.String(), "test-value")
	})

	t.Run("file preserved after show", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		}
		data, err := json.MarshalIndent(state, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))

		fileStore := file.NewStoreWithPath(path)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashShowRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err = runner.Run(t.Context(), cli.StashShowOptions{})
		require.NoError(t, err)

		// File should still exist
		_, err = os.Stat(path)
		assert.NoError(t, err)
	})
}

