package file

import (
	"errors"
	"io"
	"os"
	"testing"

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

func TestNewStore_UserHomeDirError(t *testing.T) {
	t.Parallel()

	// Save the original function and restore it after the test
	originalFunc := userHomeDirFunc

	defer func() { userHomeDirFunc = originalFunc }()

	// Inject error
	userHomeDirFunc = func() (string, error) {
		return "", errors.New("home directory not available")
	}

	store, err := NewStore("123456789012", "ap-northeast-1")
	assert.Nil(t, store)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get home directory")
}

func TestNewStoreWithPassphrase_UserHomeDirError(t *testing.T) {
	t.Parallel()

	// Save the original function and restore it after the test
	originalFunc := userHomeDirFunc

	defer func() { userHomeDirFunc = originalFunc }()

	// Inject error
	userHomeDirFunc = func() (string, error) {
		return "", errors.New("home directory not available")
	}

	store, err := NewStoreWithPassphrase("123456789012", "ap-northeast-1", "secret")
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

func TestWriteState_EncryptionError(t *testing.T) {
	t.Parallel()

	// Inject error into crypt's random reader
	crypt.SetRandReader(&errorReader{err: errors.New("random source unavailable")})

	defer crypt.ResetRandReader()

	tmpDir := t.TempDir()
	path := tmpDir + "/stage.json"
	store := NewStoreWithPath(path)
	store.SetPassphrase("secret") // Enable encryption

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam]["/test"] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     strPtr("value"),
	}

	err := store.WriteState(t.Context(), "", state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to encrypt state")
}

// errorReader is an io.Reader that returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (n int, err error) {
	return 0, r.err
}

var _ io.Reader = (*errorReader)(nil)

func strPtr(s string) *string {
	return &s
}
