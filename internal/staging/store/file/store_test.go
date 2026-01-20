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

	store, err := file.NewStore(staging.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_Exists(t *testing.T) {
	t.Parallel()

	t.Run("param file exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		// Create the param file
		err := os.WriteFile(filepath.Join(tmpDir, "param.json"), []byte(`{}`), 0o600)
		require.NoError(t, err)

		exists, err := store.Exists()
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("secret file exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		// Create the secret file
		err := os.WriteFile(filepath.Join(tmpDir, "secret.json"), []byte(`{}`), 0o600)
		require.NoError(t, err)

		exists, err := store.Exists()
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("both files exist", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		// Create both files
		err := os.WriteFile(filepath.Join(tmpDir, "param.json"), []byte(`{}`), 0o600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "secret.json"), []byte(`{}`), 0o600)
		require.NoError(t, err)

		exists, err := store.Exists()
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("no files exist", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		exists, err := store.Exists()
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("stat error (not IsNotExist)", func(t *testing.T) {
		t.Parallel()

		// Create a file where we want a directory
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "not-a-dir")
		err := os.WriteFile(filePath, []byte("content"), 0o600)
		require.NoError(t, err)

		// Use the file as a directory (invalid)
		store := file.NewStoreWithDir(filePath)

		exists, err := store.Exists()
		require.Error(t, err)
		assert.False(t, exists)
		assert.Contains(t, err.Error(), "failed to check file")
	})
}

func TestNewStoreWithPassphrase(t *testing.T) {
	t.Parallel()

	store, err := file.NewStoreWithPassphrase(staging.AWSScope("123456789012", "ap-northeast-1"), "secret")
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_IsEncrypted(t *testing.T) {
	t.Parallel()

	t.Run("not encrypted", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		// Write plain JSON to param file
		err := os.WriteFile(filepath.Join(tmpDir, "param.json"), []byte(`{"version":2}`), 0o600)
		require.NoError(t, err)

		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.False(t, isEnc)
	})

	t.Run("param encrypted", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		// Write encrypted data to param file
		encrypted, err := crypt.Encrypt([]byte(`{"version":2}`), "password")
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "param.json"), encrypted, 0o600)
		require.NoError(t, err)

		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.True(t, isEnc)
	})

	t.Run("secret encrypted", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		// Write encrypted data to secret file
		encrypted, err := crypt.Encrypt([]byte(`{"version":2}`), "password")
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "secret.json"), encrypted, 0o600)
		require.NoError(t, err)

		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.True(t, isEnc)
	})

	t.Run("files not exist", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.False(t, isEnc)
	})

	t.Run("read error (not IsNotExist)", func(t *testing.T) {
		t.Parallel()

		// Create a file where we want a directory
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "not-a-dir")
		err := os.WriteFile(filePath, []byte("content"), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithDir(filePath)

		isEnc, err := store.IsEncrypted()
		require.Error(t, err)
		assert.False(t, isEnc)
		assert.Contains(t, err.Error(), "failed to read file")
	})
}

func TestStore_Drain(t *testing.T) {
	t.Parallel()

	t.Run("empty files", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.True(t, state.IsEmpty())
	})

	t.Run("with param data keep=true", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// Write test data to param file
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
		err := os.WriteFile(filepath.Join(tmpDir, "param.json"), []byte(testData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithDir(tmpDir)
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)

		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Equal(t, "test", lo.FromPtr(state.Entries[staging.ServiceParam]["/app/config"].Value))

		// File should still exist
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		assert.NoError(t, err)
	})

	t.Run("with data keep=false", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// Write test data to both files
		paramData := `{"version": 2, "entries": {"param": {"/test": {"operation": "create"}}, "secret": {}}, "tags": {"param": {}, "secret": {}}}`
		//nolint:gosec // G101: This is test data, not an actual secret
		secretData := `{"version": 2, "entries": {"param": {}, "secret": {"mysecret": {"operation": "create"}}}, "tags": {"param": {}, "secret": {}}}`
		err := os.WriteFile(filepath.Join(tmpDir, "param.json"), []byte(paramData), 0o600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "secret.json"), []byte(secretData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithDir(tmpDir)
		_, err = store.Drain(t.Context(), "", false)
		require.NoError(t, err)

		// Both files should be deleted
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(filepath.Join(tmpDir, "secret.json"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("encrypted with passphrase", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// Write encrypted data to param file
		testData := `{"version": 2, "entries": {"param": {"/test": {"operation": "create", "value": "secret"}}, ` +
			`"secret": {}}, "tags": {"param": {}, "secret": {}}}`
		encrypted, err := crypt.Encrypt([]byte(testData), "mypassword")
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "param.json"), encrypted, 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithDir(tmpDir)
		store.SetPassphrase("mypassword")

		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "secret", lo.FromPtr(state.Entries[staging.ServiceParam]["/test"].Value))
	})

	t.Run("encrypted without passphrase fails", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// Write encrypted data to param file
		encrypted, err := crypt.Encrypt([]byte(`{"version": 2}`), "mypassword")
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "param.json"), encrypted, 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithDir(tmpDir)
		_, err = store.Drain(t.Context(), "", true)
		assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
	})

	t.Run("with service filter - param", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// Write test data to param file
		paramData := `{
			"version": 2,
			"entries": {
				"param": {
					"/app/config": {"operation": "update", "value": "param-val"}
				},
				"secret": {}
			},
			"tags": {"param": {}, "secret": {}}
		}`
		err := os.WriteFile(filepath.Join(tmpDir, "param.json"), []byte(paramData), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithDir(tmpDir)

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

		store := file.NewStoreWithDir(filePath)

		_, err = store.Drain(t.Context(), "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read param file")
	})

	t.Run("JSON parse error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// Write invalid JSON to param file
		err := os.WriteFile(filepath.Join(tmpDir, "param.json"), []byte(`{invalid json`), 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithDir(tmpDir)
		_, err = store.Drain(t.Context(), "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse param file")
	})

	t.Run("encrypted with wrong passphrase", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// Write encrypted data to param file
		encrypted, err := crypt.Encrypt([]byte(`{"version": 2}`), "correct-password")
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "param.json"), encrypted, 0o600)
		require.NoError(t, err)

		store := file.NewStoreWithDir(tmpDir)
		store.SetPassphrase("wrong-password")

		_, err = store.Drain(t.Context(), "", true)
		assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
	})
}

func TestStore_Persist(t *testing.T) {
	t.Parallel()

	t.Run("persist param state", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
		}

		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// Param file should exist
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		require.NoError(t, err)

		// Read back and verify
		readState, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "test-value", lo.FromPtr(readState.Entries[staging.ServiceParam]["/app/config"].Value))
	})

	t.Run("persist empty state removes files", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

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

		// Param file should be removed
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("persist with encryption", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)
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
		store := file.NewStoreWithDir(tmpDir)

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

		// Only param file should exist
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(tmpDir, "secret.json"))
		assert.True(t, os.IsNotExist(err))

		// Read back and verify only param was persisted
		readState, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Len(t, readState.Entries[staging.ServiceParam], 1)
		assert.Empty(t, readState.Entries[staging.ServiceSecret])
	})

	t.Run("persist creates directory if not exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "nested", "dir")
		store := file.NewStoreWithDir(nestedDir)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test"),
		}

		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// File should exist
		_, err = os.Stat(filepath.Join(nestedDir, "param.json"))
		assert.NoError(t, err)
	})

	t.Run("persist directory creation error", func(t *testing.T) {
		t.Parallel()

		// Create a file where we want a directory
		tmpDir := t.TempDir()
		blocker := filepath.Join(tmpDir, "blocker")
		err := os.WriteFile(blocker, []byte("content"), 0o600)
		require.NoError(t, err)

		// Try to use the "blocker" file as a directory
		store := file.NewStoreWithDir(blocker)

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
		store := file.NewStoreWithDir(tmpDir)

		// Persist empty state - should not error even if files don't exist
		emptyState := staging.NewEmptyState()
		err := store.WriteState(t.Context(), "", emptyState)
		require.NoError(t, err)
	})

	t.Run("persist write error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		// Create a directory with the same name as the target file
		paramPath := filepath.Join(tmpDir, "param.json")
		err := os.MkdirAll(paramPath, 0o750)
		require.NoError(t, err)

		store := file.NewStoreWithDir(tmpDir)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test"),
		}

		err = store.WriteState(t.Context(), "", state)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write param file")
	})
}

func TestStore_Delete(t *testing.T) {
	t.Parallel()

	t.Run("delete existing files", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		// Create both files
		err := os.WriteFile(filepath.Join(tmpDir, "param.json"), []byte(`{"version":1}`), 0o600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "secret.json"), []byte(`{"version":1}`), 0o600)
		require.NoError(t, err)

		// Delete
		err = store.Delete()
		require.NoError(t, err)

		// Verify files are deleted
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(filepath.Join(tmpDir, "secret.json"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("delete non-existent files (no error)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		// Delete should not error even if files don't exist
		err := store.Delete()
		require.NoError(t, err)
	})

	t.Run("delete encrypted files", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		// Create encrypted store and write
		storeWithPass := file.NewStoreWithDir(tmpDir)
		storeWithPass.SetPassphrase("test-passphrase")

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
		}
		err := storeWithPass.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// Verify file is encrypted
		//nolint:gosec // G304: path is from t.TempDir(), safe for test
		data, err := os.ReadFile(filepath.Join(tmpDir, "param.json"))
		require.NoError(t, err)
		assert.True(t, crypt.IsEncrypted(data))

		// Create store without passphrase and delete (should still work)
		storeNoPass := file.NewStoreWithDir(tmpDir)
		err = storeNoPass.Delete()
		require.NoError(t, err)

		// Verify file is deleted
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		assert.True(t, os.IsNotExist(err))
	})
}

func TestStore_BothServices(t *testing.T) {
	t.Parallel()

	t.Run("persist and drain both services", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		}

		// Persist both services
		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// Both files should exist
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(tmpDir, "secret.json"))
		require.NoError(t, err)

		// Drain all and verify
		readState, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Len(t, readState.Entries[staging.ServiceParam], 1)
		assert.Len(t, readState.Entries[staging.ServiceSecret], 1)
		assert.Equal(t, "param-value", lo.FromPtr(readState.Entries[staging.ServiceParam]["/app/config"].Value))
		assert.Equal(t, "secret-value", lo.FromPtr(readState.Entries[staging.ServiceSecret]["my-secret"].Value))
	})

	t.Run("drain specific service only", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithDir(tmpDir)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		}

		// Persist both services
		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// Drain only secret, keep=false
		secretState, err := store.Drain(t.Context(), staging.ServiceSecret, false)
		require.NoError(t, err)
		assert.Empty(t, secretState.Entries[staging.ServiceParam])
		assert.Len(t, secretState.Entries[staging.ServiceSecret], 1)

		// Secret file should be deleted, param file should remain
		_, err = os.Stat(filepath.Join(tmpDir, "param.json"))
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(tmpDir, "secret.json"))
		assert.True(t, os.IsNotExist(err))
	})
}
