// In-package tests of the core Store's key resolution.
//declscope:core

package file

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
	"github.com/mpyw/suve/internal/staging/store/file/internal/keyprovider"
)

func newTestKey() []byte {
	key := make([]byte, crypt.RawKeyLen)
	for i := range key {
		key[i] = byte(i + 1)
	}

	return key
}

func TestStore_KeyRoundTrip(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	store := NewStoreWithPath(path)
	store.key = newTestKey()

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("raw-key-value"),
	}

	require.NoError(t, store.WriteState(t.Context(), "", state))

	// File must be encrypted using the raw-key (v3) format.
	raw, err := os.ReadFile(path) //nolint:gosec // test temp path
	require.NoError(t, err)
	require.True(t, crypt.IsEncrypted(raw))
	assert.Equal(t, crypt.VersionRawKeyAAD, raw[len(crypt.MagicHeader)])

	// Read back with the same key.
	got, err := store.Drain(t.Context(), "", true)
	require.NoError(t, err)
	assert.Equal(t, "raw-key-value", lo.FromPtr(got.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}].Value))
}

func TestStore_KeyReadsLegacyPlaintext(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// A legacy, unencrypted stage.json must still be readable by a
	// key-configured store (migration path).
	plain := `{"version":3,"entries":{"param":[{"name":"/legacy","operation":"create","value":"plain"}]}}`
	require.NoError(t, os.WriteFile(path, []byte(plain), 0o600))

	store := NewStoreWithPath(path)
	store.key = newTestKey()

	got, err := store.Drain(t.Context(), "", true)
	require.NoError(t, err)
	assert.Equal(t, "plain", lo.FromPtr(got.Entries[staging.ServiceParam][staging.EntryKey{Name: "/legacy"}].Value))
}

func TestStore_KeyWrongKeyFails(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	store := NewStoreWithPath(path)
	store.key = newTestKey()

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("v"),
	}
	require.NoError(t, store.WriteState(t.Context(), "", state))

	// A store with a different key cannot decrypt.
	other := NewStoreWithPath(path)
	wrong := newTestKey()
	wrong[0] ^= 0xff
	other.key = wrong

	_, err := other.Drain(t.Context(), "", true)
	require.ErrorIs(t, err, crypt.ErrKeyMismatch)
	assert.Contains(t, err.Error(), "SUVE_STAGING_KEY")
	assert.NotContains(t, err.Error(), "passphrase")
}

// TestNewWorkingStore_KeyConfigured verifies the constructor stores the
// resolved key when the provider returns one.
//
//nolint:paralleltest // overrides package-level resolveKeyFunc.
func TestNewWorkingStore_KeyConfigured(t *testing.T) {
	origResolve := resolveKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		userHomeDirFunc = origHome
	}()

	userHomeDirFunc = func() (string, error) { return t.TempDir(), nil }

	key := newTestKey()
	resolveKeyFunc = func() ([]byte, bool, bool, error) { return key, false, false, nil }

	s, err := NewWorkingStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)
	assert.Equal(t, key, s.key)
	assert.Empty(t, s.passphrase)
}

// TestNewWorkingStore_PlaintextFallback verifies no key is configured when the
// provider falls back to plaintext.
//
//nolint:paralleltest // overrides package-level resolveKeyFunc.
func TestNewWorkingStore_PlaintextFallback(t *testing.T) {
	origResolve := resolveKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		userHomeDirFunc = origHome
	}()

	userHomeDirFunc = func() (string, error) { return t.TempDir(), nil }

	resolveKeyFunc = func() ([]byte, bool, bool, error) { return nil, true, false, nil }

	s, err := NewWorkingStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)
	assert.Nil(t, s.key)
}

// TestNewWorkingStore_Plaintext_EncryptedStateExists_Fatal guards #523: when no
// key is available on this platform (plaintext fallback) but encrypted state
// already exists, the constructor must hard-fail like the keychain-unavailable
// and lost-key branches instead of silently degrading to plaintext.
//
//nolint:paralleltest // overrides package-level resolveKeyFunc.
func TestNewWorkingStore_Plaintext_EncryptedStateExists_Fatal(t *testing.T) {
	origResolve := resolveKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		userHomeDirFunc = origHome
	}()

	home := t.TempDir()
	userHomeDirFunc = func() (string, error) { return home, nil }

	scope := provider.AWSScope("123456789012", "ap-northeast-1")

	// Seed an ENCRYPTED param.json under the scope directory.
	seed, err := NewStore(scope)
	require.NoError(t, err)

	seed.key = newTestKey()

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("v"),
	}
	require.NoError(t, seed.WriteState(t.Context(), "", state))

	resolveKeyFunc = func() ([]byte, bool, bool, error) { return nil, true, false, nil }

	s, err := NewWorkingStore(scope)
	require.Error(t, err)
	assert.Nil(t, s)
	assert.Contains(t, err.Error(), "encrypted state exists")
	assert.ErrorIs(t, err, keyprovider.ErrNoKeyAvailable)
}

// TestNewWorkingStore_ResolveError verifies a provider error propagates.
//
//nolint:paralleltest // overrides package-level resolveKeyFunc.
func TestNewWorkingStore_ResolveError(t *testing.T) {
	origResolve := resolveKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		userHomeDirFunc = origHome
	}()

	userHomeDirFunc = func() (string, error) { return t.TempDir(), nil }

	resolveKeyFunc = func() ([]byte, bool, bool, error) { return nil, false, false, errors.New("bad key") }

	s, err := NewWorkingStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.Error(t, err)
	assert.Nil(t, s)
	assert.Contains(t, err.Error(), "failed to resolve staging encryption key")
}

// TestNewWorkingStore_KeychainError_NoEncryptedState_Plaintext verifies that a
// hard keychain failure degrades to plaintext when no encrypted state exists
// yet (so the tool stays usable on e.g. headless CI without a keyring).
//
//nolint:paralleltest // overrides package-level resolveKeyFunc.
func TestNewWorkingStore_KeychainError_NoEncryptedState_Plaintext(t *testing.T) {
	origResolve := resolveKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		userHomeDirFunc = origHome
	}()

	userHomeDirFunc = func() (string, error) { return t.TempDir(), nil }
	resolveKeyFunc = func() ([]byte, bool, bool, error) {
		return nil, false, false, &keyprovider.KeychainUnavailableError{Err: errors.New("dbus down")}
	}

	s, err := NewWorkingStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)
	assert.Nil(t, s.key)
}

// TestNewWorkingStore_KeychainError_EncryptedStateExists_Fatal verifies that a
// hard keychain failure is surfaced (not downgraded) when encrypted state
// already exists — the real cause must reach the user instead of a later
// misleading "wrong passphrase" decryption error.
//
//nolint:paralleltest // overrides package-level resolveKeyFunc.
func TestNewWorkingStore_KeychainError_EncryptedStateExists_Fatal(t *testing.T) {
	origResolve := resolveKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		userHomeDirFunc = origHome
	}()

	home := t.TempDir()
	userHomeDirFunc = func() (string, error) { return home, nil }

	scope := provider.AWSScope("123456789012", "ap-northeast-1")

	// Seed an ENCRYPTED param.json under the scope directory.
	seed, err := NewStore(scope)
	require.NoError(t, err)

	seed.key = newTestKey()

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("v"),
	}
	require.NoError(t, seed.WriteState(t.Context(), "", state))

	resolveKeyFunc = func() ([]byte, bool, bool, error) {
		return nil, false, false, &keyprovider.KeychainUnavailableError{Err: errors.New("keychain locked")}
	}

	s, err := NewWorkingStore(scope)
	require.Error(t, err)
	assert.Nil(t, s)
	assert.Contains(t, err.Error(), "keychain locked")
	assert.Contains(t, err.Error(), "encrypted state exists")
}

// TestNewWorkingStore_NeedsMint_NoEncryptedState_Mints verifies that on a
// genuine first run (keychain reachable but empty, no encrypted state) the
// constructor mints a key and configures it.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestNewWorkingStore_NeedsMint_NoEncryptedState_Mints(t *testing.T) {
	origResolve := resolveKeyFunc
	origMint := mintKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		mintKeyFunc = origMint
		userHomeDirFunc = origHome
	}()

	userHomeDirFunc = func() (string, error) { return t.TempDir(), nil }
	resolveKeyFunc = func() ([]byte, bool, bool, error) { return nil, false, true, nil }

	key := newTestKey()

	minted := false
	mintKeyFunc = func() ([]byte, error) {
		minted = true

		return key, nil
	}

	s, err := NewWorkingStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)
	assert.True(t, minted, "expected a key to be minted on first run")
	assert.Equal(t, key, s.key)
}

// TestNewWorkingStore_NeedsMint_EncryptedStateExists_Fatal guards #475: if the
// keychain entry is lost while encrypted working state persists, the next
// constructor call must surface the real cause and MUST NOT mint (and thereby
// silently store) a replacement key that could never decrypt that state.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestNewWorkingStore_NeedsMint_EncryptedStateExists_Fatal(t *testing.T) {
	origResolve := resolveKeyFunc
	origMint := mintKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		mintKeyFunc = origMint
		userHomeDirFunc = origHome
	}()

	home := t.TempDir()
	userHomeDirFunc = func() (string, error) { return home, nil }

	scope := provider.AWSScope("123456789012", "ap-northeast-1")

	// Seed an ENCRYPTED param.json under the scope directory, as if written
	// earlier with a now-lost key.
	seed, err := NewStore(scope)
	require.NoError(t, err)

	seed.key = newTestKey()

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/secret"}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("v"),
	}
	require.NoError(t, seed.WriteState(t.Context(), "", state))

	// The keychain now reports the key as absent (deleted/reset).
	resolveKeyFunc = func() ([]byte, bool, bool, error) { return nil, false, true, nil }

	minted := false
	mintKeyFunc = func() ([]byte, error) {
		minted = true

		return newTestKey(), nil
	}

	s, err := NewWorkingStore(scope)
	require.Error(t, err)
	assert.Nil(t, s)
	assert.False(t, minted, "must not mint a replacement key when encrypted state exists")
	assert.Contains(t, err.Error(), "encrypted state exists")
	assert.ErrorIs(t, err, keyprovider.ErrKeychainKeyNotFound)
}

// TestWriteFileAtomic guards #325: writes go through a temp file + rename, so
// the target ends up complete, owner-only, and no temp file is left behind.
func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "stage.json")

	require.NoError(t, writeFileAtomic(path, []byte("hello")))

	got, err := os.ReadFile(path) //nolint:gosec // test temp path
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))

	// No temp files left behind in the directory.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "stage.json", entries[0].Name())

	// Owner-only permissions.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// TestLockPath verifies the advisory lockfile path in both store modes (#326).
func TestLockPath(t *testing.T) {
	t.Parallel()

	split, err := NewStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(split.stateDir, ".lock"), split.lockPath())

	single := NewStoreWithPath(filepath.Join(t.TempDir(), "single.json"))
	assert.Equal(t, filepath.Join(filepath.Dir(single.stateFilePath), ".lock"), single.lockPath())
}

// TestLock_CreatesLockfileAndOperationsWork verifies a mutating operation
// acquires the file lock (creating the lockfile) and still persists correctly
// (#326).
func TestLock_CreatesLockfileAndOperationsWork(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStoreWithPath(filepath.Join(dir, "stage.json"))

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/a"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("v"),
	}))

	// The advisory lockfile was created in the state file's directory.
	_, statErr := os.Stat(filepath.Join(dir, ".lock"))
	require.NoError(t, statErr)

	got, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/a", Namespace: ""})
	require.NoError(t, err)
	assert.Equal(t, "v", lo.FromPtr(got.Value))
}

// TestNewWorkingStore_KeyResolutionUsesGlobalLock covers #999: the data key
// lives in one global keychain slot, so its Resolve/Mint sequence must be
// serialized across scopes, not per scope directory. Another holder of the
// staging-root key lock (standing in for a process working in a different
// scope) must block key resolution until it releases the lock.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestNewWorkingStore_KeyResolutionUsesGlobalLock(t *testing.T) {
	origResolve := resolveKeyFunc
	origHome := userHomeDirFunc

	defer func() {
		resolveKeyFunc = origResolve
		userHomeDirFunc = origHome
	}()

	home := t.TempDir()
	userHomeDirFunc = func() (string, error) { return home, nil }

	resolved := make(chan struct{}, 1)
	resolveKeyFunc = func() ([]byte, bool, bool, error) {
		resolved <- struct{}{}

		return newTestKey(), false, false, nil
	}

	// Hold the key lock through a separate file handle, as another process
	// staging in another scope would.
	lockDir := filepath.Join(home, baseDirName, stagingDir)
	require.NoError(t, os.MkdirAll(lockDir, 0o700))

	other := flock.New(filepath.Join(lockDir, keyLockFileName))
	require.NoError(t, other.Lock())

	done := make(chan error, 1)

	go func() {
		_, err := NewWorkingStore(provider.GoogleCloudScope("my-project"))
		done <- err
	}()

	select {
	case <-resolved:
		t.Fatal("key resolution ran while another scope held the key lock")
	case <-time.After(300 * time.Millisecond):
	}

	require.NoError(t, other.Unlock())

	select {
	case <-resolved:
	case <-time.After(5 * time.Second):
		t.Fatal("key resolution did not run after the key lock was released")
	}

	require.NoError(t, <-done)
}

// TestStore_KeyBindsScopeAndService covers #1007: a working file encrypted with
// the data key is bound to its scope and service, so the same key cannot read
// it after it is copied into another scope's directory or swapped with the
// other service's file.
//
//nolint:paralleltest // newSplitStore overrides package-level userHomeDirFunc.
func TestStore_KeyBindsScopeAndService(t *testing.T) {
	src := newSplitStore(t)
	key := staging.EntryKey{Name: "/app/secret"}

	require.NoError(t, src.StageEntry(t.Context(), staging.ServiceParam, key, staging.Entry{
		Operation: staging.OperationUpdate, Value: new("v"),
	}))

	// Same scope, same key: readable.
	_, err := src.GetEntry(t.Context(), staging.ServiceParam, key)
	require.NoError(t, err)

	raw, err := os.ReadFile(src.servicePath(staging.ServiceParam))
	require.NoError(t, err)

	t.Run("copied to another scope", func(t *testing.T) {
		dst, err := NewStore(provider.AWSScope("210987654321", "ap-northeast-1"))
		require.NoError(t, err)

		dst.key = src.key

		require.NoError(t, os.MkdirAll(dst.stateDir, 0o700))
		require.NoError(t, os.WriteFile(dst.servicePath(staging.ServiceParam), raw, 0o600)) //nolint:gosec // test temp path

		_, err = dst.GetEntry(t.Context(), staging.ServiceParam, key)
		require.ErrorIs(t, err, crypt.ErrKeyMismatch)
	})

	t.Run("swapped with the other service", func(t *testing.T) {
		require.NoError(t, os.WriteFile(src.servicePath(staging.ServiceSecret), raw, 0o600)) //nolint:gosec // test temp path

		_, err := src.GetEntry(t.Context(), staging.ServiceSecret, key)
		require.ErrorIs(t, err, crypt.ErrKeyMismatch)
	})
}

// TestStore_KeyWarnsOnPlaintextFile covers #1007: an unencrypted working file
// read while a data key is configured is still read (and re-encrypted on the
// next write) but reported once, since suve never writes it that way itself.
//
//nolint:paralleltest // overrides package-level userHomeDirFunc and the warn sink.
func TestStore_KeyWarnsOnPlaintextFile(t *testing.T) {
	s := newSplitStore(t)
	key := staging.EntryKey{Name: "/app/planted"}

	var buf bytes.Buffer

	prev := SetWarnWriter(&buf)
	t.Cleanup(func() { SetWarnWriter(prev) })

	plain := NewStoreWithPath(s.servicePath(staging.ServiceParam))
	require.NoError(t, plain.StageEntry(t.Context(), staging.ServiceParam, key, staging.Entry{
		Operation: staging.OperationUpdate, Value: new("injected"),
	}))

	for range 2 {
		entry, err := s.GetEntry(t.Context(), staging.ServiceParam, key)
		require.NoError(t, err)
		assert.Equal(t, "injected", lo.FromPtr(entry.Value))
	}

	assert.Equal(t, 1, strings.Count(buf.String(), "is not encrypted although an encryption key is configured"),
		"warned once per file: %q", buf.String())
	assert.Contains(t, buf.String(), s.servicePath(staging.ServiceParam))

	// The next write encrypts it.
	require.NoError(t, s.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/other"}, staging.Entry{
		Operation: staging.OperationUpdate, Value: new("v"),
	}))

	raw, err := os.ReadFile(s.servicePath(staging.ServiceParam))
	require.NoError(t, err)
	assert.True(t, crypt.IsEncrypted(raw))
}
