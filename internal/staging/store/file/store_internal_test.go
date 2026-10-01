// In-package tests of the core Store (the file-name namespace storeInternal
// names no unit of its own).
//declscope:core

package file

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

// updateTestEntry builds a minimal staged entry for the Update tests.
func updateTestEntry(value string) staging.Entry {
	return staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new(value),
		StagedAt:  time.Now(),
	}
}

func TestStore_Update_ReadModifyWrite(t *testing.T) {
	t.Parallel()

	s := NewStoreWithPath(filepath.Join(t.TempDir(), "stage.json"))

	keyA := staging.EntryKey{Name: "/a"}
	require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, keyA, updateTestEntry("a")))

	keyB := staging.EntryKey{Name: "/b"}

	require.NoError(t, s.Update(t.Context(), "", func(st *staging.State) error {
		st.Entries[staging.ServiceParam][keyB] = updateTestEntry("b")

		return nil
	}))

	final, err := s.Drain(t.Context(), "", true)
	require.NoError(t, err)
	assert.Contains(t, final.Entries[staging.ServiceParam], keyA)
	assert.Contains(t, final.Entries[staging.ServiceParam], keyB)
}

func TestStore_Update_FnErrorLeavesStateUnchanged(t *testing.T) {
	t.Parallel()

	s := NewStoreWithPath(filepath.Join(t.TempDir(), "stage.json"))

	keyA := staging.EntryKey{Name: "/a"}
	require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, keyA, updateTestEntry("a")))

	sentinel := errors.New("boom")
	err := s.Update(t.Context(), "", func(st *staging.State) error {
		st.Entries[staging.ServiceParam][staging.EntryKey{Name: "/b"}] = updateTestEntry("b")

		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	// The failed mutation must not have been persisted.
	final, err := s.Drain(t.Context(), "", true)
	require.NoError(t, err)
	assert.Len(t, final.Entries[staging.ServiceParam], 1)
	assert.Contains(t, final.Entries[staging.ServiceParam], keyA)
}

// TestStore_Update_AtomicAgainstConcurrentStage proves the read-modify-write
// cycle is atomic: a StageEntry from a second store handle (standing in for
// another process) that fires while Update holds the lock cannot interleave, so
// neither the Update's change nor the concurrent stage is lost. Under the old
// Drain + WriteState(stale snapshot) pattern the concurrent entry was silently
// clobbered.
//
//nolint:paralleltest // mutates the package-level updateMidHook; must run serially
func TestStore_Update_AtomicAgainstConcurrentStage(t *testing.T) {
	// Not parallel: it sets the package-level updateMidHook.
	dir := t.TempDir()
	path := filepath.Join(dir, "stage.json")

	writer := NewStoreWithPath(path) // the Update caller (import path)
	stager := NewStoreWithPath(path) // a second handle standing in for another process

	keyA := staging.EntryKey{Name: "/a"}
	keyB := staging.EntryKey{Name: "/b"}
	keyC := staging.EntryKey{Name: "/c"}

	require.NoError(t, writer.StageEntry(t.Context(), staging.ServiceParam, keyA, updateTestEntry("a")))

	stageStarted := make(chan struct{})
	stageDone := make(chan struct{})

	var stageErr error

	updateMidHook = func() {
		go func() {
			close(stageStarted)
			// Blocks on the store lock (held by Update) until the cycle completes.
			stageErr = stager.StageEntry(context.Background(), staging.ServiceParam, keyC, updateTestEntry("c"))

			close(stageDone)
		}()

		<-stageStarted

		// The concurrent stage must NOT complete while Update holds the lock.
		select {
		case <-stageDone:
			t.Error("concurrent StageEntry completed while Update held the store lock")
		case <-time.After(50 * time.Millisecond):
		}
	}

	t.Cleanup(func() { updateMidHook = nil })

	require.NoError(t, writer.Update(t.Context(), "", func(st *staging.State) error {
		st.Entries[staging.ServiceParam][keyB] = updateTestEntry("b")

		return nil
	}))

	<-stageDone
	require.NoError(t, stageErr)

	final, err := writer.Drain(t.Context(), "", true)
	require.NoError(t, err)

	// Nothing lost: pre-existing A, the Update's B, and the concurrent C all survive.
	assert.Contains(t, final.Entries[staging.ServiceParam], keyA)
	assert.Contains(t, final.Entries[staging.ServiceParam], keyB)
	assert.Contains(t, final.Entries[staging.ServiceParam], keyC)
}

func TestInitializeStateMaps(t *testing.T) {
	t.Parallel()

	t.Run("nil entries", func(t *testing.T) {
		t.Parallel()

		state := &staging.State{
			Entries: nil,
			Tags:    nil,
		}

		initializeStateMaps(state)

		assert.NotNil(t, state.Entries)
		assert.NotNil(t, state.Entries[staging.ServiceParam])
		assert.NotNil(t, state.Entries[staging.ServiceSecret])
		assert.NotNil(t, state.Tags)
		assert.NotNil(t, state.Tags[staging.ServiceParam])
		assert.NotNil(t, state.Tags[staging.ServiceSecret])
	})

	t.Run("empty entries map", func(t *testing.T) {
		t.Parallel()

		state := &staging.State{
			Entries: make(map[staging.Service]map[staging.EntryKey]staging.Entry),
			Tags:    make(map[staging.Service]map[staging.EntryKey]staging.TagEntry),
		}

		initializeStateMaps(state)

		assert.NotNil(t, state.Entries[staging.ServiceParam])
		assert.NotNil(t, state.Entries[staging.ServiceSecret])
		assert.NotNil(t, state.Tags[staging.ServiceParam])
		assert.NotNil(t, state.Tags[staging.ServiceSecret])
	})

	t.Run("partial entries map", func(t *testing.T) {
		t.Parallel()

		state := &staging.State{
			Entries: map[staging.Service]map[staging.EntryKey]staging.Entry{
				staging.ServiceParam: {staging.EntryKey{Name: "key"}: staging.Entry{}},
			},
			Tags: map[staging.Service]map[staging.EntryKey]staging.TagEntry{
				staging.ServiceSecret: {staging.EntryKey{Name: "key"}: staging.TagEntry{}},
			},
		}

		initializeStateMaps(state)

		// Should preserve existing data
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Len(t, state.Tags[staging.ServiceSecret], 1)

		// Should initialize missing maps
		assert.NotNil(t, state.Entries[staging.ServiceSecret])
		assert.NotNil(t, state.Tags[staging.ServiceParam])
	})

	t.Run("already initialized", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "key"}] = staging.Entry{}
		state.Tags[staging.ServiceSecret][staging.EntryKey{Name: "key"}] = staging.TagEntry{}

		initializeStateMaps(state)

		// Should preserve all existing data
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Len(t, state.Tags[staging.ServiceSecret], 1)
	})
}

// Note: This test cannot use t.Parallel() because it modifies the global userHomeDirFunc variable.
//
//nolint:paralleltest // Modifies package-level variable userHomeDirFunc.
func TestNewStore_UserHomeDirError(t *testing.T) {
	// Save the original function and restore it after the test
	originalFunc := userHomeDirFunc

	defer func() { userHomeDirFunc = originalFunc }()

	// Inject error
	userHomeDirFunc = func() (string, error) {
		return "", errors.New("home directory not available")
	}

	store, err := newStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	assert.Nil(t, store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get home directory")
}

// Note: This test cannot use t.Parallel() because it modifies the global userHomeDirFunc variable.
//
//nolint:paralleltest // Modifies package-level variable userHomeDirFunc.
func TestNewStoreWithPassphrase_UserHomeDirError(t *testing.T) {
	// Save the original function and restore it after the test
	originalFunc := userHomeDirFunc

	defer func() { userHomeDirFunc = originalFunc }()

	// Inject error
	userHomeDirFunc = func() (string, error) {
		return "", errors.New("home directory not available")
	}

	store, err := newStoreWithPassphrase(provider.AWSScope("123456789012", "ap-northeast-1"), "secret")
	assert.Nil(t, store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get home directory")
}

func TestDrain_RemoveFileError(t *testing.T) {
	t.Parallel()

	// This test validates the error path when os.Remove fails in Drain
	// We can trigger this by making the file unremovable
	tmpDir := t.TempDir()
	dirPath := tmpDir + "/subdir"
	err := os.MkdirAll(dirPath, 0o750)
	require.NoError(t, err)

	path := dirPath + "/stage.json"
	err = os.WriteFile(path, []byte(`{"version":2,"entries":{"param":{},"secret":{}},"tags":{"param":{},"secret":{}}}`), 0o600)
	require.NoError(t, err)

	// Make directory read-only so file can't be removed
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithPath(path)

	_, err = store.Drain(t.Context(), "", false) // keep=false triggers remove
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove state file")
}

func TestWriteState_RemoveEmptyStateError(t *testing.T) {
	t.Parallel()

	// Create a directory structure where we can't remove the file
	tmpDir := t.TempDir()
	dirPath := tmpDir + "/subdir"
	err := os.MkdirAll(dirPath, 0o750)
	require.NoError(t, err)

	path := dirPath + "/stage.json"
	err = os.WriteFile(path, []byte(`{}`), 0o600)
	require.NoError(t, err)

	// Make directory read-only so file can't be removed
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithPath(path)

	// Empty state should trigger file removal, which should fail
	emptyState := staging.NewEmptyState()
	err = store.WriteState(t.Context(), "", emptyState)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove empty state file")
}

// Note: This test cannot use t.Parallel() because it modifies the global randReader variable in crypt package.
//
//nolint:paralleltest // Modifies package-level variable via crypt.SetRandReader.
func TestWriteState_EncryptionError(t *testing.T) {
	// Inject error into crypt's random reader
	crypt.SetRandReader(&errorReader{err: errors.New("random source unavailable")})

	defer crypt.ResetRandReader()

	tmpDir := t.TempDir()
	path := tmpDir + "/stage.json"
	store := NewStoreWithPath(path)
	store.SetPassphrase("secret") // Enable encryption

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/test"}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("value"),
	}

	err := store.WriteState(t.Context(), "", state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to encrypt state")
}

// TestReadFile_EmptyOrWhitespaceTreatedAsEmpty guards #562: a state file that is
// zero bytes or contains only whitespace (e.g. an external truncation) must be
// read as an empty state instead of hard-failing every command with a parse
// error, mirroring how a missing file is handled.
func TestReadFile_EmptyOrWhitespaceTreatedAsEmpty(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"zero bytes":       "",
		"single newline":   "\n",
		"spaces and tabs":  "  \t  ",
		"crlf and spaces":  " \r\n\t ",
		"trailing newline": "\n\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "stage.json")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

			store := NewStoreWithPath(path)

			state, err := store.Drain(t.Context(), "", true)
			require.NoError(t, err)
			assert.True(t, state.IsEmpty(), "trimmed-empty file should read as empty state")
		})
	}
}

// TestReadFile_NonEmptyGarbageStillErrors verifies the empty-file tolerance does
// not swallow genuinely corrupt (non-empty) content.
func TestReadFile_NonEmptyGarbageStillErrors(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stage.json")
	require.NoError(t, os.WriteFile(path, []byte("not json at all"), 0o600))

	store := NewStoreWithPath(path)

	_, err := store.Drain(t.Context(), "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

// errorReader is an io.Reader that returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (n int, err error) {
	return 0, r.err
}

var _ io.Reader = (*errorReader)(nil)

func TestNewStore(t *testing.T) {
	t.Parallel()

	store, err := newStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_Exists(t *testing.T) {
	t.Parallel()

	t.Run("file exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := NewStoreWithPath(path)

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
		store := NewStoreWithPath(path)

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
		store := NewStoreWithPath(invalidPath)

		exists, err := store.Exists()
		require.Error(t, err)
		assert.False(t, exists)
		assert.Contains(t, err.Error(), "failed to check state file")
	})
}

func TestNewStoreWithPassphrase(t *testing.T) {
	t.Parallel()

	store, err := newStoreWithPassphrase(provider.AWSScope("123456789012", "ap-northeast-1"), "secret")
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_IsEncrypted(t *testing.T) {
	t.Parallel()

	t.Run("not encrypted", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := NewStoreWithPath(path)

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
		store := NewStoreWithPath(path)

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
		store := NewStoreWithPath(path)

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
		store := NewStoreWithPath(invalidPath)

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
		path := filepath.Join(tmpDir, "stage.json")
		store := NewStoreWithPath(path)

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
			"version": 3,
			"entries": {
				"param": [
					{"name": "/app/config", "operation": "update", "value": "test"}
				]
			}
		}`
		err := os.WriteFile(path, []byte(testData), 0o600)
		require.NoError(t, err)

		store := NewStoreWithPath(path)
		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)

		assert.Equal(t, 3, state.Version)
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Equal(t, "test", lo.FromPtr(state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))

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

		store := NewStoreWithPath(path)
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
		testData := `{"version": 3, "entries": {"param": [{"name": "/test", "operation": "create", "value": "secret"}]}}`
		encrypted, err := crypt.Encrypt([]byte(testData), "mypassword")
		require.NoError(t, err)
		err = os.WriteFile(path, encrypted, 0o600)
		require.NoError(t, err)

		// Create store with custom path and passphrase for test
		store := NewStoreWithPath(path)
		store.SetPassphrase("mypassword")

		state, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "secret", lo.FromPtr(state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/test"}].Value))
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

		store := NewStoreWithPath(path)
		_, err = store.Drain(t.Context(), "", true)
		assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
	})

	t.Run("with service filter", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write test data with both services
		testData := `{
			"version": 3,
			"entries": {
				"param": [
					{"name": "/app/config", "operation": "update", "value": "param-val"}
				],
				"secret": [
					{"name": "my-secret", "operation": "create", "value": "secret-val"}
				]
			}
		}`
		err := os.WriteFile(path, []byte(testData), 0o600)
		require.NoError(t, err)

		store := NewStoreWithPath(path)

		// Drain only param service
		state, err := store.Drain(t.Context(), staging.ServiceParam, true)
		require.NoError(t, err)

		// Should only have param entries
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Empty(t, state.Entries[staging.ServiceSecret])
		assert.Equal(t, "param-val", lo.FromPtr(state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))
	})

	t.Run("read error (not IsNotExist)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "not-a-dir")
		err := os.WriteFile(filePath, []byte("content"), 0o600)
		require.NoError(t, err)

		invalidPath := filepath.Join(filePath, "stage.json")
		store := NewStoreWithPath(invalidPath)

		_, err = store.Drain(t.Context(), "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read state file")
	})

	t.Run("JSON parse error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Write invalid JSON
		err := os.WriteFile(path, []byte(`{invalid json`), 0o600)
		require.NoError(t, err)

		store := NewStoreWithPath(path)
		_, err = store.Drain(t.Context(), "", true)
		require.Error(t, err)
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

		store := NewStoreWithPath(path)
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
		store := NewStoreWithPath(path)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("test-value"),
		}

		err := store.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// File should exist
		_, err = os.Stat(path)
		require.NoError(t, err)

		// Read back and verify
		readState, err := store.Drain(t.Context(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "test-value", lo.FromPtr(readState.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))
	})

	t.Run("persist empty state removes file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := NewStoreWithPath(path)

		// First persist non-empty state
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("test"),
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
		store := NewStoreWithPath(path)
		store.SetPassphrase("secret123")

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     new("encrypted-value"),
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
		assert.Equal(t, "encrypted-value", lo.FromPtr(readState.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}].Value))
	})

	t.Run("persist with service filter", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := NewStoreWithPath(path)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("param-value"),
		}
		state.Entries[staging.ServiceSecret][staging.EntryKey{Name: "my-secret"}] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     new("secret-value"),
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
		store := NewStoreWithPath(nestedPath)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("test"),
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
		store := NewStoreWithPath(invalidPath)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("test"),
		}

		err = store.WriteState(t.Context(), "", state)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create state directory")
	})

	t.Run("persist empty state removes non-existent file gracefully", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "nonexistent.json")
		store := NewStoreWithPath(path)

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

		store := NewStoreWithPath(filePath)

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("test"),
		}

		err = store.WriteState(t.Context(), "", state)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write state file")
	})
}

func TestStore_Delete(t *testing.T) {
	t.Parallel()

	t.Run("delete existing file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")
		store := NewStoreWithPath(path)

		// Create the file
		err := os.WriteFile(path, []byte(`{"version":1}`), 0o600)
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(path)
		require.NoError(t, err)

		// Delete
		err = store.Delete()
		require.NoError(t, err)

		// Verify file is deleted
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("delete non-existent file (no error)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "nonexistent.json")
		store := NewStoreWithPath(path)

		// Delete should not error even if file doesn't exist
		err := store.Delete()
		require.NoError(t, err)
	})

	t.Run("delete encrypted file", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "stage.json")

		// Create encrypted store
		storeWithPass := NewStoreWithPath(path)
		storeWithPass.SetPassphrase("test-passphrase")

		// Write encrypted state
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     new("secret-value"),
		}
		err := storeWithPass.WriteState(t.Context(), "", state)
		require.NoError(t, err)

		// Verify file is encrypted
		data, err := os.ReadFile(path) //nolint:gosec // Test file path from temp directory
		require.NoError(t, err)
		assert.True(t, crypt.IsEncrypted(data))

		// Create store without passphrase and delete
		storeNoPass := NewStoreWithPath(path)
		err = storeNoPass.Delete()
		require.NoError(t, err)

		// Verify file is deleted
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})
}
