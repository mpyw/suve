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

func TestGlobalDropRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("success - drop all services", func(t *testing.T) {
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

		runner := &cli.GlobalDropRunner{
			FileStore: fileStore,
			Stdout:    stdout,
		}

		err = runner.Run()
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "All stashed changes dropped")

		// File should be deleted
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("success - drop non-existent file (no error)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		// Don't create the file

		fileStore := file.NewStoreWithPath(path)
		stdout := &bytes.Buffer{}

		runner := &cli.GlobalDropRunner{
			FileStore: fileStore,
			Stdout:    stdout,
		}

		// GlobalDropRunner.Run does not check existence, it just deletes
		err := runner.Run()
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "All stashed changes dropped")
	})
}

func TestServiceDropRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("success - drop specific service", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data with both services
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
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

		runner := &cli.ServiceDropRunner{
			FileStore: fileStore,
			Service:   staging.ServiceParam,
			Stdout:    stdout,
		}

		// Drop only param service
		err = runner.Run(t.Context())
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Stashed param changes dropped")

		// File should still exist with secret service
		_, err = os.Stat(path)
		require.NoError(t, err)

		// Verify secret service is preserved
		remainingState, err := fileStore.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Empty(t, remainingState.Entries[staging.ServiceParam])
		assert.Len(t, remainingState.Entries[staging.ServiceSecret], 1)
	})

	t.Run("success - drop service with tags only", func(t *testing.T) {
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

		runner := &cli.ServiceDropRunner{
			FileStore: fileStore,
			Service:   staging.ServiceParam,
			Stdout:    stdout,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Stashed param changes dropped")

		// File should be deleted (empty state)
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
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

		runner := &cli.ServiceDropRunner{
			FileStore: fileStore,
			Service:   staging.ServiceSecret,
			Stdout:    stdout,
		}

		// Try to drop secret service which has no entries
		err = runner.Run(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no stashed changes for secret")
	})

	t.Run("success - drop last service deletes file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data with only one service
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

		runner := &cli.ServiceDropRunner{
			FileStore: fileStore,
			Service:   staging.ServiceParam,
			Stdout:    stdout,
		}

		// Drop the only service
		err = runner.Run(t.Context())
		require.NoError(t, err)

		// File should be deleted because state is now empty
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("file preserved after drop specific service", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data with both services
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
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

		runner := &cli.ServiceDropRunner{
			FileStore: fileStore,
			Service:   staging.ServiceParam,
			Stdout:    stdout,
		}

		err = runner.Run(t.Context())
		require.NoError(t, err)

		// File should still exist
		_, err = os.Stat(path)
		require.NoError(t, err)

		// Read and verify remaining data
		//nolint:gosec // G304: path is from t.TempDir(), safe for test
		remainingData, err := os.ReadFile(path)
		require.NoError(t, err)

		var remainingState staging.State
		require.NoError(t, json.Unmarshal(remainingData, &remainingState))
		assert.Empty(t, remainingState.Entries[staging.ServiceParam])
		assert.Len(t, remainingState.Entries[staging.ServiceSecret], 1)
	})
}

func TestState_TotalCount(t *testing.T) {
	t.Parallel()

	t.Run("nil state", func(t *testing.T) {
		t.Parallel()

		var s *staging.State
		assert.Equal(t, 0, s.TotalCount())
	})

	t.Run("empty state", func(t *testing.T) {
		t.Parallel()

		s := staging.NewEmptyState()
		assert.Equal(t, 0, s.TotalCount())
	})

	t.Run("entries only", func(t *testing.T) {
		t.Parallel()

		s := staging.NewEmptyState()
		s.Entries[staging.ServiceParam]["/app/config1"] = staging.Entry{}
		s.Entries[staging.ServiceParam]["/app/config2"] = staging.Entry{}
		s.Entries[staging.ServiceSecret]["secret1"] = staging.Entry{}
		assert.Equal(t, 3, s.TotalCount())
	})

	t.Run("tags only", func(t *testing.T) {
		t.Parallel()

		s := staging.NewEmptyState()
		s.Tags[staging.ServiceParam]["/app/config"] = staging.TagEntry{}
		s.Tags[staging.ServiceSecret]["secret"] = staging.TagEntry{}
		assert.Equal(t, 2, s.TotalCount())
	})

	t.Run("entries and tags", func(t *testing.T) {
		t.Parallel()

		s := staging.NewEmptyState()
		s.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{}
		s.Tags[staging.ServiceParam]["/app/config2"] = staging.TagEntry{}
		s.Entries[staging.ServiceSecret]["secret"] = staging.Entry{}
		assert.Equal(t, 3, s.TotalCount())
	})
}
