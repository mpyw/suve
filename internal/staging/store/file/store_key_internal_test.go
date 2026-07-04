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
	state.Entries[staging.ServiceParam]["/app/secret"] = staging.Entry{
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
	assert.Equal(t, "raw-key-value", lo.FromPtr(got.Entries[staging.ServiceParam]["/app/secret"].Value))
}

func TestStore_KeyReadsLegacyPlaintext(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// A legacy, unencrypted stage.json must still be readable by a
	// key-configured store (migration path).
	plain := `{"version":2,"entries":{"param":{"/legacy":{"operation":"create","value":"plain"}},"secret":{}},"tags":{"param":{},"secret":{}}}`
	require.NoError(t, os.WriteFile(path, []byte(plain), 0o600))

	store := NewStoreWithPath(path)
	store.key = newTestKey()

	got, err := store.Drain(t.Context(), "", true)
	require.NoError(t, err)
	assert.Equal(t, "plain", lo.FromPtr(got.Entries[staging.ServiceParam]["/legacy"].Value))
}

func TestStore_KeyWrongKeyFails(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	store := NewStoreWithPath(path)
	store.key = newTestKey()

	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam]["/app/secret"] = staging.Entry{
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
