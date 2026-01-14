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

	store, err := NewStore("123456789012", "ap-northeast-1", staging.ServiceParam)
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

	store, err := NewStoreWithPassphrase("123456789012", "ap-northeast-1", staging.ServiceParam, "secret")
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

	path := dirPath + "/param.json"
	err = os.WriteFile(path, []byte(`{"version":3,"entries":{"param":{}},"tags":{"param":{}}}`), 0o600)
	require.NoError(t, err)

	// Make directory read-only so file can't be removed
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithPath(path, staging.ServiceParam)

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

	path := dirPath + "/param.json"
	err = os.WriteFile(path, []byte(`{"version":3,"entries":{"param":{}},"tags":{"param":{}}}`), 0o600)
	require.NoError(t, err)

	// Make directory read-only so file can't be removed
	//nolint:gosec // G302: intentionally restrictive permissions for test
	err = os.Chmod(dirPath, 0o555)
	require.NoError(t, err)
	//nolint:gosec // G302: restore permissions for cleanup
	defer func() { _ = os.Chmod(dirPath, 0o755) }()

	store := NewStoreWithPath(path, staging.ServiceParam)

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
	path := tmpDir + "/param.json"
	store := NewStoreWithPath(path, staging.ServiceParam)
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
