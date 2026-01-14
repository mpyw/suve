package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/file"
)

func TestStashDropRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("success - drop service file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")

		// Write V3 format test data
		testData := `{"version":3,"service":"param","entries":{"/app/config":{"operation":"update","value":"test-value"}}}`
		require.NoError(t, os.WriteFile(path, []byte(testData), 0o600))

		fileStore := file.NewStoreWithPath(path, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashDropRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context())
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Stashed param changes dropped")

		// File should be deleted
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("error - no stashed changes to drop", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		// Don't create the file

		fileStore := file.NewStoreWithPath(path, staging.ServiceParam)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashDropRunner{
			FileStore: fileStore,
			Stdout:    stdout,
			Stderr:    stderr,
		}

		err := runner.Run(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no stashed changes to drop")
	})
}

func TestGlobalStashDropRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("success - drop all services", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")
		secretPath := filepath.Join(tmpDir, "secret.json")

		// Write V3 format test data for both services
		paramData := `{"version":3,"service":"param","entries":{"/app/config":{"operation":"update","value":"param-value"}}}`
		//nolint:gosec // G101: test data, not actual credentials
		secretData := `{"version":3,"service":"secret","entries":{"my-secret":{"operation":"create","value":"secret-value"}}}`

		require.NoError(t, os.WriteFile(paramPath, []byte(paramData), 0o600))
		require.NoError(t, os.WriteFile(secretPath, []byte(secretData), 0o600))

		fileStores := map[staging.Service]*file.Store{
			staging.ServiceParam:  file.NewStoreWithPath(paramPath, staging.ServiceParam),
			staging.ServiceSecret: file.NewStoreWithPath(secretPath, staging.ServiceSecret),
		}
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.GlobalStashDropRunner{
			FileStores: fileStores,
			Stdout:     stdout,
			Stderr:     stderr,
		}

		err := runner.Run(t.Context())
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "All stashed changes dropped")

		// Both files should be deleted
		_, err = os.Stat(paramPath)
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(secretPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("success - drop with only one service file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")
		secretPath := filepath.Join(tmpDir, "secret.json")

		// Only create param file
		paramData := `{"version":3,"service":"param","entries":{"/app/config":{"operation":"update","value":"param-value"}}}`
		require.NoError(t, os.WriteFile(paramPath, []byte(paramData), 0o600))

		fileStores := map[staging.Service]*file.Store{
			staging.ServiceParam:  file.NewStoreWithPath(paramPath, staging.ServiceParam),
			staging.ServiceSecret: file.NewStoreWithPath(secretPath, staging.ServiceSecret),
		}
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.GlobalStashDropRunner{
			FileStores: fileStores,
			Stdout:     stdout,
			Stderr:     stderr,
		}

		err := runner.Run(t.Context())
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "All stashed changes dropped")

		// Param file should be deleted
		_, err = os.Stat(paramPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("error - no stashed changes to drop", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		paramPath := filepath.Join(tmpDir, "param.json")
		secretPath := filepath.Join(tmpDir, "secret.json")
		// Don't create any files

		fileStores := map[staging.Service]*file.Store{
			staging.ServiceParam:  file.NewStoreWithPath(paramPath, staging.ServiceParam),
			staging.ServiceSecret: file.NewStoreWithPath(secretPath, staging.ServiceSecret),
		}
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.GlobalStashDropRunner{
			FileStores: fileStores,
			Stdout:     stdout,
			Stderr:     stderr,
		}

		err := runner.Run(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no stashed changes to drop")
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
