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
}

func TestStore_Drain(t *testing.T) {
	t.Parallel()

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := file.NewStoreWithPath(path)

		state, err := store.Drain(t.Context(), true)
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
		state, err := store.Drain(t.Context(), true)
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
		_, err = store.Drain(t.Context(), false)
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
		testData := `{"version": 2, "entries": {"param": {"/test": {"operation": "create", "value": "secret"}}, "secret": {}}, "tags": {"param": {}, "secret": {}}}`
		encrypted, err := crypt.Encrypt([]byte(testData), "mypassword")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		// Create store with custom path and passphrase for test
		store := file.NewStoreWithPath(path)
		store.SetPassphrase("mypassword")

		state, err := store.Drain(t.Context(), true)
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
		_, err = store.Drain(t.Context(), true)
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

		err := store.WriteState(t.Context(), state)
		require.NoError(t, err)

		// File should exist
		_, err = os.Stat(path)
		assert.NoError(t, err)

		// Read back and verify
		readState, err := store.Drain(t.Context(), true)
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
		err := store.WriteState(t.Context(), state)
		require.NoError(t, err)

		// Then persist empty state
		emptyState := staging.NewEmptyState()
		err = store.WriteState(t.Context(), emptyState)
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

		err := store.WriteState(t.Context(), state)
		require.NoError(t, err)

		// File should be encrypted
		isEnc, err := store.IsEncrypted()
		require.NoError(t, err)
		assert.True(t, isEnc)

		// Should be able to drain with same passphrase
		readState, err := store.Drain(t.Context(), true)
		require.NoError(t, err)
		assert.Equal(t, "encrypted-value", lo.FromPtr(readState.Entries[staging.ServiceParam]["/app/secret"].Value))
	})
}
