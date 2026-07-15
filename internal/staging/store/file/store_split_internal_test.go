package file

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file/internal/keyprovider"
)

// newSplitStore builds a split (working) Store for a param+secret AWS scope
// under a fresh temp HOME, with a raw data key configured so state files are
// encrypted (mirroring the SUVE_STAGING_KEY keychain-bypass path). It overrides
// the package-level userHomeDirFunc, so callers must NOT run in parallel.
func newSplitStore(t *testing.T) *Store {
	t.Helper()

	home := t.TempDir()

	orig := userHomeDirFunc
	userHomeDirFunc = func() (string, error) { return home, nil }

	t.Cleanup(func() { userHomeDirFunc = orig })

	s, err := NewStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)

	s.key = newTestKey()

	return s
}

// makeReadOnly chmods dir to 0o500 and restores 0o700 on cleanup so t.TempDir's
// own teardown can still delete it.
func makeReadOnly(t *testing.T, dir string) {
	t.Helper()

	//nolint:gosec // G302: intentionally restrictive permissions for the test
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() {
		//nolint:gosec // G302: restore permissions so TempDir cleanup succeeds
		_ = os.Chmod(dir, 0o700)
	})
}

// TestStore_Exists_Split exercises the split-mode multi-service loop: it must
// stat every supported service's file and short-circuit on the first hit,
// including when only the second service (secret) is present, and propagate a
// non-IsNotExist stat error.
//
//nolint:paralleltest // newSplitStore overrides package-level userHomeDirFunc
func TestStore_Exists_Split(t *testing.T) {
	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("no files", func(t *testing.T) {
		s := newSplitStore(t)

		ok, err := s.Exists()
		require.NoError(t, err)
		assert.False(t, ok)
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("param file present", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, updateTestEntry("v")))

		ok, err := s.Exists()
		require.NoError(t, err)
		assert.True(t, ok)
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("only secret present iterates past param", func(t *testing.T) {
		s := newSplitStore(t)

		// Staging only a secret leaves param.json absent, so Exists must keep
		// looping past the missing param file and find secret.json.
		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, updateTestEntry("v")))

		ok, err := s.Exists()
		require.NoError(t, err)
		assert.True(t, ok)
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("stat error propagates", func(t *testing.T) {
		s := newSplitStore(t)

		// Make the scope directory a regular file so statting a service file
		// under it fails with a non-IsNotExist error.
		require.NoError(t, os.MkdirAll(filepath.Dir(s.stateDir), 0o700))
		require.NoError(t, os.WriteFile(s.stateDir, []byte("x"), 0o600))

		ok, err := s.Exists()
		require.Error(t, err)
		assert.False(t, ok)
		assert.Contains(t, err.Error(), "failed to check state file")
	})
}

// TestStore_Delete_Split covers the split-mode delete loop across both services,
// the absent-file no-op, and the os.Remove non-IsNotExist error branch.
//
//nolint:paralleltest // newSplitStore overrides package-level userHomeDirFunc
func TestStore_Delete_Split(t *testing.T) {
	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("removes all service files", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, updateTestEntry("p")))
		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, updateTestEntry("s")))

		require.NoError(t, s.Delete())

		_, err := os.Stat(s.servicePath(staging.ServiceParam))
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(s.servicePath(staging.ServiceSecret))
		assert.True(t, os.IsNotExist(err))
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("no files is a no-op", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.Delete())
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("remove error propagates", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, updateTestEntry("p")))
		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, updateTestEntry("s")))

		// A read-only scope directory prevents removing the service files.
		makeReadOnly(t, s.stateDir)

		err := s.Delete()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove state file")
	})
}

// TestStore_UnstageAll_Split covers the split-mode clear loop for all services,
// a single-service clear, and the readFile error branch.
//
//nolint:paralleltest // newSplitStore overrides package-level userHomeDirFunc
func TestStore_UnstageAll_Split(t *testing.T) {
	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("clears all services and removes files", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, updateTestEntry("p")))
		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, updateTestEntry("s")))

		require.NoError(t, s.UnstageAll(t.Context(), ""))

		_, err := os.Stat(s.servicePath(staging.ServiceParam))
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(s.servicePath(staging.ServiceSecret))
		assert.True(t, os.IsNotExist(err))
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("clears a single service and leaves the other", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, updateTestEntry("p")))
		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, updateTestEntry("s")))

		require.NoError(t, s.UnstageAll(t.Context(), staging.ServiceParam))

		_, err := s.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"})
		require.ErrorIs(t, err, staging.ErrNotStaged)

		got, err := s.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"})
		require.NoError(t, err)
		assert.Equal(t, "s", *got.Value)
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("read error propagates", func(t *testing.T) {
		s := newSplitStore(t)

		// A directory in place of param.json makes readFile fail with a
		// non-IsNotExist error, which UnstageAll must surface.
		require.NoError(t, os.MkdirAll(s.servicePath(staging.ServiceParam), 0o700))

		err := s.UnstageAll(t.Context(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read state file")
	})
}

// TestStore_Drain_Split covers drainServiceFile via the split-mode Drain loop:
// merging all services, filtering to one service, and the os.Remove error path.
//
//nolint:paralleltest // newSplitStore overrides package-level userHomeDirFunc
func TestStore_Drain_Split(t *testing.T) {
	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("drains and removes all services", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, updateTestEntry("p")))
		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, updateTestEntry("s")))

		state, err := s.Drain(t.Context(), "", false)
		require.NoError(t, err)
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Len(t, state.Entries[staging.ServiceSecret], 1)

		_, err = os.Stat(s.servicePath(staging.ServiceParam))
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(s.servicePath(staging.ServiceSecret))
		assert.True(t, os.IsNotExist(err))
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("drains a single service", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, updateTestEntry("p")))
		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, updateTestEntry("s")))

		state, err := s.Drain(t.Context(), staging.ServiceSecret, false)
		require.NoError(t, err)
		assert.Empty(t, state.Entries[staging.ServiceParam])
		assert.Len(t, state.Entries[staging.ServiceSecret], 1)

		// Only the secret file was drained/removed; param survives.
		_, err = os.Stat(s.servicePath(staging.ServiceSecret))
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(s.servicePath(staging.ServiceParam))
		require.NoError(t, err)
	})

	//nolint:paralleltest // shares the userHomeDirFunc override
	t.Run("remove error propagates", func(t *testing.T) {
		s := newSplitStore(t)

		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, updateTestEntry("p")))
		require.NoError(t, s.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, updateTestEntry("s")))

		makeReadOnly(t, s.stateDir)

		_, err := s.Drain(t.Context(), "", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove state file")
	})
}

// TestStore_WriteState_Split_WriteError covers the error return inside the
// split-mode writeStateLocked loop: a read-only scope directory makes the atomic
// write fail on the first service.
//
//nolint:paralleltest // newSplitStore overrides package-level userHomeDirFunc
func TestStore_WriteState_Split_WriteError(t *testing.T) {
	s := newSplitStore(t)

	require.NoError(t, os.MkdirAll(s.stateDir, 0o700))
	makeReadOnly(t, s.stateDir)

	err := s.WriteState(t.Context(), "", nonEmptyState())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write state file")
}

// TestWriteFileAtomic_Failures covers the writeFileAtomic failure branches that
// can be forced deterministically: CreateTemp (missing parent directory) and
// Rename (target path already occupied by a directory). Both must leave no temp
// file behind.
func TestWriteFileAtomic_Failures(t *testing.T) {
	t.Parallel()

	t.Run("create temp error on missing parent", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "missing-dir", "stage.json")

		err := writeFileAtomic(path, []byte("data"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create temp state file")
	})

	t.Run("rename error when target is a directory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "target")
		require.NoError(t, os.Mkdir(path, 0o700))

		err := writeFileAtomic(path, []byte("data"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to rename temp state file")

		// The temp file must be cleaned up: only the target directory remains.
		entries, readErr := os.ReadDir(dir)
		require.NoError(t, readErr)
		require.Len(t, entries, 1)
		assert.Equal(t, "target", entries[0].Name())
	})
}

// TestNewWorkingStore_EncryptionCheckError covers the guardKeyLossWithEncryptedState
// probe-failure branch: a hard keychain error triggers the guard, and if the
// encryption probe itself fails (a directory sits where param.json should be),
// the constructor surfaces that check error.
//
//nolint:paralleltest // overrides package-level resolveKeyFunc/userHomeDirFunc
func TestNewWorkingStore_EncryptionCheckError(t *testing.T) {
	origResolve := resolveKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		userHomeDirFunc = origHome
	}()

	home := t.TempDir()
	userHomeDirFunc = func() (string, error) { return home, nil }

	scope := provider.AWSScope("123456789012", "ap-northeast-1")

	probe, err := NewStore(scope)
	require.NoError(t, err)

	// A directory where param.json belongs makes isFileEncrypted's ReadFile fail
	// with a non-IsNotExist error, so IsEncrypted (and thus the guard) errors.
	require.NoError(t, os.MkdirAll(probe.servicePath(staging.ServiceParam), 0o700))

	resolveKeyFunc = func() ([]byte, bool, bool, error) {
		return nil, false, false, &keyprovider.KeychainUnavailableError{Err: errors.New("keychain down")}
	}

	s, err := NewWorkingStore(scope)
	require.Error(t, err)
	assert.Nil(t, s)
	assert.Contains(t, err.Error(), "failed to check staging state encryption")
}

// TestWarnPlaintextOnce_WithCause covers the branch that appends a non-nil
// underlying cause to the one-time plaintext warning, and pins that the warning
// is written to the redirectable sink (SetWarnWriter) rather than straight to
// os.Stderr — the seam the TUI uses to keep the warning out of its alt-screen.
// It resets the package once-guard (sync.Once cannot be copied, so it is reset
// to a fresh value rather than saved/restored) so the warning body runs
// regardless of test ordering.
//
//nolint:paralleltest // mutates the package-level plaintextWarnOnce guard
func TestWarnPlaintextOnce_WithCause(t *testing.T) {
	plaintextWarnOnce = sync.Once{}

	var buf bytes.Buffer

	prev := SetWarnWriter(&buf)
	defer SetWarnWriter(prev)

	warnPlaintextOnce(errors.New("keychain unavailable"))

	out := buf.String()
	assert.Contains(t, out, "stored UNENCRYPTED", "the primary warning must reach the sink")
	assert.Contains(t, out, "keychain unavailable", "the underlying cause must be appended")
}

// TestSetWarnWriter_SwapAndRestore pins that SetWarnWriter returns the previous
// writer so a caller (tui.Run) can capture during a program and restore after.
func TestSetWarnWriter_SwapAndRestore(t *testing.T) { //nolint:paralleltest // mutates package-level warnWriter
	var a, b bytes.Buffer

	orig := SetWarnWriter(&a)
	defer SetWarnWriter(orig)

	got := SetWarnWriter(&b)
	assert.Same(t, &a, got, "SetWarnWriter must return the writer it replaced")
}
