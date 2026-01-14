package file_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

func TestNewStore(t *testing.T) {
	t.Parallel()

	store, err := file.NewStore("123456789012", "ap-northeast-1", staging.ServiceParam)
	require.NoError(t, err)
	assert.NotNil(t, store)
	assert.Equal(t, staging.ServiceParam, store.Service())
}

func TestStore_Exists(t *testing.T) {
	t.Parallel()

	t.Run("file exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		// Create the file
		err := os.WriteFile(path, []byte(`{}`), 0o600)
		require.NoError(t, err)

		exists, err := store.Exists()
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("file does not exist", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "nonexistent.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		exists, err := store.Exists()
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("stat error (not IsNotExist)", func(t *testing.T) {
		t.Parallel()

		// Create a directory, then create a file inside, and try to stat a path
		// that goes through the file as if it were a directory
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "not-a-dir")
		err := os.WriteFile(filePath, []byte("content"), 0o600)
		require.NoError(t, err)

		// Try to stat a path through the file (which is not a directory)
		invalidPath := filepath.Join(filePath, "param.json")
		store := file.NewStoreWithPath(invalidPath, staging.ServiceParam)

		exists, err := store.Exists()
		require.Error(t, err)
		assert.False(t, exists)
		assert.Contains(t, err.Error(), "failed to check state file")
	})
}

func TestNewStoreWithPassphrase(t *testing.T) {
	t.Parallel()

	store, err := file.NewStoreWithPassphrase("123456789012", "ap-northeast-1", staging.ServiceParam, "secret")
	require.NoError(t, err)
	assert.NotNil(t, store)
	assert.Equal(t, staging.ServiceParam, store.Service())
}

func TestStore_IsEncrypted(t *testing.T) {
	t.Parallel()

	t.Run("not encrypted", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		// Write plain JSON (V3 format)
		err := os.WriteFile(path, []byte(`{"version":3,"service":"param"}`), 0o600)
		require.NoError(t, err)

		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.False(t, isEnc)
	})

	t.Run("encrypted", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		// Write encrypted data
		encrypted, err := crypt.Encrypt([]byte(`{"version":3,"service":"param"}`), "password")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.True(t, isEnc)
	})

	t.Run("file not exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "nonexistent.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.False(t, isEnc)
	})

	t.Run("read error (not IsNotExist)", func(t *testing.T) {
		t.Parallel()

		// Create a path through a file (not a directory) to trigger read error
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "not-a-dir")
		err := os.WriteFile(filePath, []byte("content"), 0o600)
		require.NoError(t, err)

		invalidPath := filepath.Join(filePath, "param.json")
		store := file.NewStoreWithPath(invalidPath, staging.ServiceParam)

		isEnc, err := store.IsEncrypted()
		require.Error(t, err)
		assert.False(t, isEnc)
		assert.Contains(t, err.Error(), "failed to read state file")
	})
}

func TestStore_Drain(t *testing.T) {
	t.Parallel()

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.True(t, state.IsEmpty())
	})

	t.Run("with data keep=true", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")

		// Write test data (V3 format)
		testData := `{
			"version": 3,
			"service": "param",
			"entries": {
				"/app/config": {"operation": "update", "value": "test"}
			}
		}`
		err := os.WriteFile(path, []byte(testData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path, staging.ServiceParam)
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)

		assert.Equal(t, 3, state.Version)
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Equal(t, "test", lo.FromPtr(state.Entries[staging.ServiceParam]["/app/config"].Value))

		// File should still exist
		_, err = os.Stat(path)
		assert.NoError(t, err)
	})

	t.Run("with data keep=false", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")

		// Write test data (V3 format)
		testData := `{"version": 3, "service": "param", "entries": {}}`
		err := os.WriteFile(path, []byte(testData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path, staging.ServiceParam)
		_, err = store.Drain(t.Context(), "", false)
		require.NoError(t, err)

		// File should be deleted
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("encrypted with passphrase", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")

		// Write encrypted data (V3 format)
		testData := `{"version": 3, "service": "param", "entries": {"/test": {"operation": "create", "value": "secret"}}}`
		encrypted, err := crypt.Encrypt([]byte(testData), "mypassword")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		// Create store with custom path and passphrase for test
		store := file.NewStoreWithPath(path, staging.ServiceParam)
		store.SetPassphrase("mypassword")

		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "secret", lo.FromPtr(state.Entries[staging.ServiceParam]["/test"].Value))
	})

	t.Run("encrypted without passphrase fails", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")

		// Write encrypted data (V3 format)
		encrypted, err := crypt.Encrypt([]byte(`{"version": 3, "service": "param"}`), "mypassword")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path, staging.ServiceParam)
		_, err = store.Drain(t.Context(), "", true)
		assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
	})

	t.Run("service mismatch returns empty", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")

		// Write test data (V3 format)
		testData := `{"version": 3, "service": "param", "entries": {"/app/config": {"operation": "update", "value": "test"}}}`
		err := os.WriteFile(path, []byte(testData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path, staging.ServiceParam)

		// Request secret service from a param store - should return empty (no-op)
		state, err := store.Drain(t.Context(), staging.ServiceSecret, true)
		require.NoError(t, err)
		assert.True(t, state.IsEmpty())
	})

	t.Run("read error (not IsNotExist)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "not-a-dir")
		err := os.WriteFile(filePath, []byte("content"), 0o600)
		require.NoError(t, err)

		invalidPath := filepath.Join(filePath, "param.json")
		store := file.NewStoreWithPath(invalidPath, staging.ServiceParam)

		_, err = store.Drain(t.Context(), "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read state file")
	})

	t.Run("JSON parse error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")

		// Write invalid JSON
		err := os.WriteFile(path, []byte(`{invalid json`), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path, staging.ServiceParam)
		_, err = store.Drain(t.Context(), "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse state file")
	})

	t.Run("encrypted with wrong passphrase", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")

		// Write encrypted data (V3 format)
		encrypted, err := crypt.Encrypt([]byte(`{"version": 3, "service": "param"}`), "correct-password")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path, staging.ServiceParam)
		store.SetPassphrase("wrong-password")

		_, err = store.Drain(t.Context(), "", true)
		assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
	})
}

func TestStore_Persist(t *testing.T) {
	t.Parallel()

	t.Run("persist state", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
		}

		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// File should exist
		_, err = os.Stat(path)
		require.NoError(t, err)

		// Read back and verify
		readState, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "test-value", lo.FromPtr(readState.Entries[staging.ServiceParam]["/app/config"].Value))
	})

	t.Run("persist empty state removes file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		// First persist non-empty state
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test"),
		}
		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// Then persist empty state
		emptyState := staging.NewEmptyState()
		err = store.WriteState(t.Context(), "", emptyState)
		require.NoError(t, err)

		// File should be removed
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("persist with encryption", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)
		store.SetPassphrase("secret123")

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("encrypted-value"),
		}

		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// File should be encrypted
		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.True(t, isEnc)

		// Should be able to drain with same passphrase
		readState, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "encrypted-value", lo.FromPtr(readState.Entries[staging.ServiceParam]["/app/secret"].Value))
	})

	t.Run("service mismatch is no-op", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "param.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		}

		// Request to persist secret service to a param store - should be no-op
		err := store.WriteState(t.Context(), staging.ServiceSecret, state)
		require.NoError(t, err)

		// File should not exist (no-op)
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("persist creates directory if not exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		nestedPath := filepath.Join(tmpDir, "nested", "dir", "param.json")
		store := file.NewStoreWithPath(nestedPath, staging.ServiceParam)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test"),
		}

		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// File should exist
		_, err = os.Stat(nestedPath)
		assert.NoError(t, err)
	})

	t.Run("persist directory creation error", func(t *testing.T) {
		t.Parallel()

		// Create a file where we want a directory
		tmpDir := t.TempDir()
		blocker := filepath.Join(tmpDir, "blocker")
		err := os.WriteFile(blocker, []byte("content"), 0o600)
		require.NoError(t, err)

		// Try to create file inside the "blocker" file (as if it were a directory)
		invalidPath := filepath.Join(blocker, "nested", "param.json")
		store := file.NewStoreWithPath(invalidPath, staging.ServiceParam)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test"),
		}

		err = store.WriteState(t.Context(), "", state)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create state directory")
	})

	t.Run("persist empty state removes non-existent file gracefully", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "nonexistent.json")
		store := file.NewStoreWithPath(path, staging.ServiceParam)

		// Persist empty state - should not error even if file doesn't exist
		emptyState := staging.NewEmptyState()
		err := store.WriteState(t.Context(), "", emptyState)
		require.NoError(t, err)
	})

	t.Run("persist write error", func(t *testing.T) {
		t.Parallel()

		// Create a directory where the file should be - WriteFile will fail
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "param.json")

		// Create a directory with the same name as the target file
		err := os.MkdirAll(filePath, 0o750)
		require.NoError(t, err)

		store := file.NewStoreWithPath(filePath, staging.ServiceParam)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test"),
		}

		err = store.WriteState(t.Context(), "", state)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write state file")
	})
}
