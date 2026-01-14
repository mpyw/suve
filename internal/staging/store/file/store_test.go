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

	store, err := file.NewStore("123456789012", "ap-northeast-1")
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_Exists(t *testing.T) {
	t.Parallel()

	t.Run("file exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)

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
		store := file.NewStoreWithPath(path)

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
		invalidPath := filepath.Join(filePath, "stage.json")
		store := file.NewStoreWithPath(invalidPath)

		exists, err := store.Exists()
		assert.Error(t, err)
		assert.False(t, exists)
		assert.Contains(t, err.Error(), "failed to check state file")
	})
}

func TestNewStoreWithPassphrase(t *testing.T) {
	t.Parallel()

	store, err := file.NewStoreWithPassphrase("123456789012", "ap-northeast-1", "secret")
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_IsEncrypted(t *testing.T) {
	t.Parallel()

	t.Run("not encrypted", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)

		// Write plain JSON
		err := os.WriteFile(path, []byte(`{"version":2}`), 0o600)
		require.NoError(t, err)

		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.False(t, isEnc)
	})

	t.Run("encrypted", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)

		// Write encrypted data
		encrypted, err := crypt.Encrypt([]byte(`{"version":2}`), "password")
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
		store := file.NewStoreWithPath(path)

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

		invalidPath := filepath.Join(filePath, "stage.json")
		store := file.NewStoreWithPath(invalidPath)

		isEnc, err := store.IsEncrypted()
		assert.Error(t, err)
		assert.False(t, isEnc)
		assert.Contains(t, err.Error(), "failed to read state file")
	})
}

func TestStore_Drain(t *testing.T) {
	t.Parallel()

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)

		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.True(t, state.IsEmpty())
	})

	t.Run("with data keep=true", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data
		testData := `{
			"version": 2,
			"entries": {
				"param": {
					"/app/config": {"operation": "update", "value": "test"}
				},
				"secret": {}
			},
			"tags": {"param": {}, "secret": {}}
		}`
		err := os.WriteFile(path, []byte(testData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path)
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)

		assert.Equal(t, 2, state.Version)
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Equal(t, "test", lo.FromPtr(state.Entries[staging.ServiceParam]["/app/config"].Value))

		// File should still exist
		_, err = os.Stat(path)
		assert.NoError(t, err)
	})

	t.Run("with data keep=false", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data
		testData := `{"version": 2, "entries": {"param": {}, "secret": {}}, "tags": {"param": {}, "secret": {}}}`
		err := os.WriteFile(path, []byte(testData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path)
		_, err = store.Drain(t.Context(), "", false)
		require.NoError(t, err)

		// File should be deleted
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("encrypted with passphrase", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write encrypted data
		//nolint:lll // mock function signature
		testData := `{"version": 2, "entries": {"param": {"/test": {"operation": "create", "value": "secret"}}, "secret": {}}, "tags": {"param": {}, "secret": {}}}`
		encrypted, err := crypt.Encrypt([]byte(testData), "mypassword")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		// Create store with custom path and passphrase for test
		store := file.NewStoreWithPath(path)
		store.SetPassphrase("mypassword")

		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "secret", lo.FromPtr(state.Entries[staging.ServiceParam]["/test"].Value))
	})

	t.Run("encrypted without passphrase fails", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write encrypted data
		encrypted, err := crypt.Encrypt([]byte(`{"version": 2}`), "mypassword")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path)
		_, err = store.Drain(t.Context(), "", true)
		assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
	})

	t.Run("with service filter", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data with both services
		testData := `{
			"version": 2,
			"entries": {
				"param": {
					"/app/config": {"operation": "update", "value": "param-val"}
				},
				"secret": {
					"my-secret": {"operation": "create", "value": "secret-val"}
				}
			},
			"tags": {"param": {}, "secret": {}}
		}`
		err := os.WriteFile(path, []byte(testData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path)

		// Drain only param service
		state, err := store.Drain(t.Context(), staging.ServiceParam, true)
		require.NoError(t, err)

		// Should only have param entries
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Empty(t, state.Entries[staging.ServiceSecret])
		assert.Equal(t, "param-val", lo.FromPtr(state.Entries[staging.ServiceParam]["/app/config"].Value))
	})

	t.Run("read error (not IsNotExist)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "not-a-dir")
		err := os.WriteFile(filePath, []byte("content"), 0o600)
		require.NoError(t, err)

		invalidPath := filepath.Join(filePath, "stage.json")
		store := file.NewStoreWithPath(invalidPath)

		_, err = store.Drain(t.Context(), "", true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read state file")
	})

	t.Run("JSON parse error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write invalid JSON
		err := os.WriteFile(path, []byte(`{invalid json`), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path)
		_, err = store.Drain(t.Context(), "", true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse state file")
	})

	t.Run("encrypted with wrong passphrase", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write encrypted data
		encrypted, err := crypt.Encrypt([]byte(`{"version": 2}`), "correct-password")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithPath(path)
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
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
		}

		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// File should exist
		_, err = os.Stat(path)
		assert.NoError(t, err)

		// Read back and verify
		readState, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "test-value", lo.FromPtr(readState.Entries[staging.ServiceParam]["/app/config"].Value))
	})

	t.Run("persist empty state removes file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)

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
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)
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

	t.Run("persist with service filter", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		}

		// Persist only param service
		err := store.WriteState(t.Context(), staging.ServiceParam, state)
		require.NoError(t, err)

		// Read back and verify only param was persisted
		readState, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Len(t, readState.Entries[staging.ServiceParam], 1)
		assert.Empty(t, readState.Entries[staging.ServiceSecret])
	})

	t.Run("persist creates directory if not exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		nestedPath := filepath.Join(tmpDir, "nested", "dir", "stage.json")
		store := file.NewStoreWithPath(nestedPath)

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
		invalidPath := filepath.Join(blocker, "nested", "stage.json")
		store := file.NewStoreWithPath(invalidPath)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test"),
		}

		err = store.WriteState(t.Context(), "", state)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create state directory")
	})

	t.Run("persist empty state removes non-existent file gracefully", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "nonexistent.json")
		store := file.NewStoreWithPath(path)

		// Persist empty state - should not error even if file doesn't exist
		emptyState := staging.NewEmptyState()
		err := store.WriteState(t.Context(), "", emptyState)
		require.NoError(t, err)
	})

	t.Run("persist write error", func(t *testing.T) {
		t.Parallel()

		// Create a directory where the file should be - WriteFile will fail
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "stage.json")

		// Create a directory with the same name as the target file
		err := os.MkdirAll(filePath, 0o750)
		require.NoError(t, err)

		store := file.NewStoreWithPath(filePath)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test"),
		}

		err = store.WriteState(t.Context(), "", state)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write state file")
	})
}
