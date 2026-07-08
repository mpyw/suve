package keyprovider //nolint:testpackage // tests override unexported hook vars.

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// withHooks saves and restores the package-level hook vars around a test body.
func withHooks(t *testing.T, fn func()) {
	t.Helper()

	origLookup := lookupEnvFunc
	origGet := keyringGetFunc
	origSet := keyringSetFunc

	t.Cleanup(func() {
		lookupEnvFunc = origLookup
		keyringGetFunc = origGet
		keyringSetFunc = origSet
	})

	fn()
}

//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_EnvVar_Valid(t *testing.T) {
	withHooks(t, func() {
		want := make([]byte, 32)
		for i := range want {
			want[i] = byte(i)
		}

		encoded := base64.StdEncoding.EncodeToString(want)

		lookupEnvFunc = func(key string) (string, bool) {
			if key == EnvStagingKey {
				return encoded, true
			}

			return "", false
		}

		key, plaintext, err := Resolve()
		require.NoError(t, err)
		assert.False(t, plaintext)
		assert.Equal(t, want, key)
	})
}

//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_EnvVar_InvalidBase64(t *testing.T) {
	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) {
			return "not!valid!base64!", true
		}

		_, _, err := Resolve()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "base64")
	})
}

//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_EnvVar_WrongLength(t *testing.T) {
	withHooks(t, func() {
		// 16 bytes, not 32.
		lookupEnvFunc = func(string) (string, bool) {
			return base64.StdEncoding.EncodeToString(make([]byte, 16)), true
		}

		_, _, err := Resolve()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "32 bytes")
	})
}

//nolint:paralleltest // uses global keyring.MockInit and overrides hook vars.
func TestResolve_Keychain_GetOrCreate(t *testing.T) {
	keyring.MockInit()

	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) { return "", false }
		keyringGetFunc = keyring.Get
		keyringSetFunc = keyring.Set

		// First call: no key stored -> generate and store.
		key1, plaintext, err := Resolve()
		require.NoError(t, err)
		assert.False(t, plaintext)
		assert.Len(t, key1, 32)

		// Second call: must return the same stored key.
		key2, plaintext, err := Resolve()
		require.NoError(t, err)
		assert.False(t, plaintext)
		assert.Equal(t, key1, key2)
	})
}

// TestResolve_UnsupportedPlatform_Plaintext: a platform with no keyring backend
// takes the documented plaintext fallback.
//
//nolint:paralleltest // overrides package-level hook vars.
//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_UnsupportedPlatform_Plaintext(t *testing.T) {
	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) { return "", false }
		keyringGetFunc = func(string, string) (string, error) {
			return "", keyring.ErrUnsupportedPlatform
		}

		key, plaintext, err := Resolve()
		require.NoError(t, err)
		assert.True(t, plaintext)
		assert.Nil(t, key)
	})
}

// TestResolve_HardKeychainError_Surfaced: a hard Get failure (locked keychain,
// unreachable dbus) is surfaced as a *KeychainUnavailableError, NOT silently
// downgraded to plaintext.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_HardKeychainError_Surfaced(t *testing.T) {
	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) { return "", false }
		keyringGetFunc = func(string, string) (string, error) {
			return "", errors.New("keychain locked")
		}

		key, plaintext, err := Resolve()
		require.Error(t, err)
		assert.False(t, plaintext)
		assert.Nil(t, key)

		var kcErr *KeychainUnavailableError
		require.ErrorAs(t, err, &kcErr)
	})
}

// TestResolve_SetError_Surfaced: a failed store (Get says not-found, Set fails)
// is a hard error, surfaced rather than silently downgraded.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_SetError_Surfaced(t *testing.T) {
	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) { return "", false }
		keyringGetFunc = func(string, string) (string, error) {
			return "", keyring.ErrNotFound
		}
		keyringSetFunc = func(string, string, string) error {
			return errors.New("cannot write to keychain")
		}

		_, plaintext, err := Resolve()
		require.Error(t, err)
		assert.False(t, plaintext)

		var kcErr *KeychainUnavailableError
		require.ErrorAs(t, err, &kcErr)
	})
}

// TestResolve_CorruptStoredKey_Surfaced: a corrupted stored key is a hard error.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_CorruptStoredKey_Surfaced(t *testing.T) {
	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) { return "", false }
		keyringGetFunc = func(string, string) (string, error) {
			return "!!not-base64!!", nil
		}

		_, _, err := Resolve()
		require.Error(t, err)

		var kcErr *KeychainUnavailableError
		require.ErrorAs(t, err, &kcErr)
	})
}
