package file

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

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
			Entries: make(map[staging.Service]map[string]staging.Entry),
			Tags:    make(map[staging.Service]map[string]staging.TagEntry),
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
			Entries: map[staging.Service]map[string]staging.Entry{
				staging.ServiceParam: {"key": staging.Entry{}},
			},
			Tags: map[staging.Service]map[string]staging.TagEntry{
				staging.ServiceSecret: {"key": staging.TagEntry{}},
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
		state.Entries[staging.ServiceParam]["key"] = staging.Entry{}
		state.Tags[staging.ServiceSecret]["key"] = staging.TagEntry{}

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

	store, err := NewStore(staging.AWSScope("123456789012", "ap-northeast-1"))
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

	store, err := NewStoreWithPassphrase(staging.AWSScope("123456789012", "ap-northeast-1"), "secret")
	assert.Nil(t, store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get home directory")
}

func TestDrain_RemoveFileError(t *testing.T) {
	t.Parallel()

	// This test validates the error path when os.Remove fails in Drain
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(dirPath, 0o750)
	require.NoError(t, err)

	// Write param file
	paramPath := filepath.Join(dirPath, "param.json")
	err = os.WriteFile(paramPath, []byte(`{"version":2,"entries":{"param":{},"secret":{}},"tags":{"param":{},"secret":{}}}`), 0o600)
	require.NoError(t, err)

	// Make directory read-only so file can't be removed
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithDir(dirPath)

	_, err = store.Drain(t.Context(), staging.ServiceParam, false) // keep=false triggers remove
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove param file")
}

func TestWriteState_RemoveEmptyStateError(t *testing.T) {
	t.Parallel()

	// Create a directory structure where we can't remove the file
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(dirPath, 0o750)
	require.NoError(t, err)

	// Write param file
	paramPath := filepath.Join(dirPath, "param.json")
	err = os.WriteFile(paramPath, []byte(`{}`), 0o600)
	require.NoError(t, err)

	// Make directory read-only so file can't be removed
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithDir(dirPath)

	// Empty state should trigger file removal, which should fail
	emptyState := staging.NewEmptyState()
	err = store.WriteState(t.Context(), "", emptyState)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove empty param file")
}

// Note: This test cannot use t.Parallel() because it modifies the global randReader variable in crypt package.
//
//nolint:paralleltest // Modifies package-level variable via crypt.SetRandReader.
func TestWriteState_EncryptionError(t *testing.T) {
	// Inject error into crypt's random reader
	crypt.SetRandReader(&errorReader{err: errors.New("random source unavailable")})

	defer crypt.ResetRandReader()

	tmpDir := t.TempDir()
	store := NewStoreWithDir(tmpDir)
	store.SetPassphrase("secret") // Enable encryption

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam]["/test"] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value"),
	}

	err := store.WriteState(t.Context(), "", state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to encrypt param state")
}

// errorReader is an io.Reader that returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (n int, err error) {
	return 0, r.err
}

var _ io.Reader = (*errorReader)(nil)

func TestPathForService_UnknownService(t *testing.T) {
	t.Parallel()

	store := NewStoreWithDir(t.TempDir())

	// Unknown service should return empty string
	path := store.pathForService(staging.Service("unknown"))
	assert.Empty(t, path)
}

func TestDrainService_UnknownService(t *testing.T) {
	t.Parallel()

	store := NewStoreWithDir(t.TempDir())

	// Unknown service should return empty state (path == "")
	state, err := store.drainService(staging.Service("unknown"), true)
	require.NoError(t, err)
	assert.True(t, state.IsEmpty())
}

func TestWriteService_UnknownService(t *testing.T) {
	t.Parallel()

	store := NewStoreWithDir(t.TempDir())

	// Unknown service should return nil (path == "")
	err := store.writeService(staging.Service("unknown"), staging.NewEmptyState())
	assert.NoError(t, err)
}

func TestDelete_RemoveError(t *testing.T) {
	t.Parallel()

	// Create a directory with files that can't be removed
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(dirPath, 0o750)
	require.NoError(t, err)

	// Write both files
	paramPath := filepath.Join(dirPath, "param.json")
	secretPath := filepath.Join(dirPath, "secret.json")
	err = os.WriteFile(paramPath, []byte(`{}`), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(secretPath, []byte(`{}`), 0o600)
	require.NoError(t, err)

	// Make directory read-only so files can't be removed
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithDir(dirPath)

	err = store.Delete()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove param file")
	assert.Contains(t, err.Error(), "failed to remove secret file")
}

func TestWriteService_WriteFileError(t *testing.T) {
	t.Parallel()

	// Create a read-only directory
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(dirPath, 0o750)
	require.NoError(t, err)

	// Make directory read-only so files can't be created
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithDir(dirPath)

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam]["/test"] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value"),
	}

	err = store.writeService(staging.ServiceParam, state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write param file")
}

func TestDrain_SecretStateError(t *testing.T) {
	t.Parallel()

	// Create a directory with param file readable but secret file unreadable
	tmpDir := t.TempDir()

	// Write param file
	paramPath := filepath.Join(tmpDir, "param.json")
	err := os.WriteFile(paramPath, []byte(`{"version":2,"entries":{"param":{},"secret":{}},"tags":{"param":{},"secret":{}}}`), 0o600)
	require.NoError(t, err)

	// Write secret file with invalid JSON
	secretPath := filepath.Join(tmpDir, "secret.json")
	err = os.WriteFile(secretPath, []byte(`invalid json`), 0o600)
	require.NoError(t, err)

	store := NewStoreWithDir(tmpDir)

	_, err = store.Drain(t.Context(), "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse secret file")
}

func TestWriteState_MkdirAllError(t *testing.T) {
	t.Parallel()

	// Use a path that can't be created (file instead of directory)
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file")
	err := os.WriteFile(filePath, []byte("not a directory"), 0o600)
	require.NoError(t, err)

	// Try to use the file as a directory
	store := NewStoreWithDir(filepath.Join(filePath, "subdir"))

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam]["/test"] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value"),
	}

	err = store.WriteState(t.Context(), "", state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create state directory")
}

func TestWriteState_BothServicesError(t *testing.T) {
	t.Parallel()

	// Create a read-only directory after creating it
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(dirPath, 0o750)
	require.NoError(t, err)

	// Make directory read-only so files can't be created
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithDir(dirPath)

	// State with both param and secret entries
	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam]["/test"] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("param-value"),
	}
	state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("secret-value"),
	}

	err = store.WriteState(t.Context(), "", state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "param:")
	assert.Contains(t, err.Error(), "secret:")
}
