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
		Value:     lo.ToPtr(value),
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

	store, err := NewStore(provider.AWSScope("123456789012", "ap-northeast-1"))
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

	store, err := NewStoreWithPassphrase(provider.AWSScope("123456789012", "ap-northeast-1"), "secret")
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
		Value:     strPtr("value"),
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

func strPtr(s string) *string {
	return &s
}
