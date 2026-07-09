package file

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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
		Value:     lo.ToPtr("raw-key-value"),
	}

	require.NoError(t, store.WriteState(t.Context(), "", state))

	// File must be encrypted using the raw-key (v2) format.
	raw, err := os.ReadFile(path) //nolint:gosec // test temp path
	require.NoError(t, err)
	require.True(t, crypt.IsEncrypted(raw))
	assert.Equal(t, crypt.VersionRawKey, raw[len(crypt.MagicHeader)])

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
		Value:     lo.ToPtr("v"),
	}
	require.NoError(t, store.WriteState(t.Context(), "", state))

	// A store with a different key cannot decrypt.
	other := NewStoreWithPath(path)
	wrong := newTestKey()
	wrong[0] ^= 0xff
	other.key = wrong

	_, err := other.Drain(t.Context(), "", true)
	assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
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
	resolveKeyFunc = func() ([]byte, bool, error) { return key, false, nil }

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

	resolveKeyFunc = func() ([]byte, bool, error) { return nil, true, nil }

	s, err := NewWorkingStore(provider.AWSScope("123456789012", "ap-northeast-1"))
	require.NoError(t, err)
	assert.Nil(t, s.key)
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

	resolveKeyFunc = func() ([]byte, bool, error) { return nil, false, errors.New("bad key") }

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
	resolveKeyFunc = func() ([]byte, bool, error) {
		return nil, false, &keyprovider.KeychainUnavailableError{Err: errors.New("dbus down")}
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
		Value:     lo.ToPtr("v"),
	}
	require.NoError(t, seed.WriteState(t.Context(), "", state))

	resolveKeyFunc = func() ([]byte, bool, error) {
		return nil, false, &keyprovider.KeychainUnavailableError{Err: errors.New("keychain locked")}
	}

	s, err := NewWorkingStore(scope)
	require.Error(t, err)
	assert.Nil(t, s)
	assert.Contains(t, err.Error(), "keychain locked")
	assert.Contains(t, err.Error(), "encrypted state exists")
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

	single := NewStoreWithPath(filepath.Join(t.TempDir(), "stash.json"))
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
		Value:     lo.ToPtr("v"),
	}))

	// The advisory lockfile was created in the state file's directory.
	_, statErr := os.Stat(filepath.Join(dir, ".lock"))
	require.NoError(t, statErr)

	got, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/a", Namespace: ""})
	require.NoError(t, err)
	assert.Equal(t, "v", lo.FromPtr(got.Value))
}
