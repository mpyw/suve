package keyprovider //nolint:testpackage // tests override unexported hook vars.

import (
	"encoding/base64"
	"errors"
	"testing"
	"testing/iotest"

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
	origRand := randReader

	t.Cleanup(func() {
		lookupEnvFunc = origLookup
		keyringGetFunc = origGet
		keyringSetFunc = origSet
		randReader = origRand
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

		key, plaintext, needsMint, err := Resolve()
		require.NoError(t, err)
		assert.False(t, plaintext)
		assert.False(t, needsMint)
		assert.Equal(t, want, key)
	})
}

//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_EnvVar_InvalidBase64(t *testing.T) {
	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) {
			return "not!valid!base64!", true
		}

		_, _, _, err := Resolve()
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

		_, _, _, err := Resolve()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "32 bytes")
	})
}

//nolint:paralleltest // uses global keyring.MockInit and overrides hook vars.
func TestResolve_Keychain_MintThenReuse(t *testing.T) {
	keyring.MockInit()

	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) { return "", false }
		keyringGetFunc = keyring.Get
		keyringSetFunc = keyring.Set

		// First call: no key stored yet -> caller is told to mint (Resolve does
		// not mint or store on its own).
		key0, plaintext, needsMint, err := Resolve()
		require.NoError(t, err)
		assert.False(t, plaintext)
		assert.True(t, needsMint)
		assert.Nil(t, key0)

		// Mint stores a fresh key.
		key1, err := Mint()
		require.NoError(t, err)
		assert.Len(t, key1, 32)

		// Second call: must return the same stored key, no longer needing a mint.
		key2, plaintext, needsMint, err := Resolve()
		require.NoError(t, err)
		assert.False(t, plaintext)
		assert.False(t, needsMint)
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

		key, plaintext, needsMint, err := Resolve()
		require.NoError(t, err)
		assert.True(t, plaintext)
		assert.False(t, needsMint)
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

		key, plaintext, needsMint, err := Resolve()
		require.Error(t, err)
		assert.False(t, plaintext)
		assert.False(t, needsMint)
		assert.Nil(t, key)

		var kcErr *KeychainUnavailableError
		require.ErrorAs(t, err, &kcErr)
	})
}

// TestResolve_KeyNotFound_NeedsMint: a reachable-but-empty keychain reports
// needsMint (Resolve neither mints nor stores) so the caller can gate minting
// on the absence of encrypted state.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_KeyNotFound_NeedsMint(t *testing.T) {
	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) { return "", false }
		keyringGetFunc = func(string, string) (string, error) {
			return "", keyring.ErrNotFound
		}

		setCalled := false
		keyringSetFunc = func(string, string, string) error {
			setCalled = true

			return nil
		}

		key, plaintext, needsMint, err := Resolve()
		require.NoError(t, err)
		assert.False(t, plaintext)
		assert.True(t, needsMint)
		assert.Nil(t, key)
		assert.False(t, setCalled, "Resolve must not store a key")
	})
}

// TestMint_SetError: a failed store surfaces from Mint as an error.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestMint_SetError(t *testing.T) {
	withHooks(t, func() {
		keyringSetFunc = func(string, string, string) error {
			return errors.New("cannot write to keychain")
		}

		_, err := Mint()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "keychain")
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

		_, _, _, err := Resolve()
		require.Error(t, err)

		var kcErr *KeychainUnavailableError
		require.ErrorAs(t, err, &kcErr)
	})
}

// TestResolve_StoredKeyWrongLength_Surfaced: a stored key that decodes cleanly
// but is not exactly 32 bytes is a hard error, never a silent plaintext
// downgrade — a wrong-length key could not decrypt existing working state.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestResolve_StoredKeyWrongLength_Surfaced(t *testing.T) {
	withHooks(t, func() {
		lookupEnvFunc = func(string) (string, bool) { return "", false }
		// Valid base64 that decodes to 31 bytes, one short of keyLen.
		keyringGetFunc = func(string, string) (string, error) {
			return base64.StdEncoding.EncodeToString(make([]byte, keyLen-1)), nil
		}

		_, _, _, err := Resolve()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wrong length")

		var kcErr *KeychainUnavailableError
		require.ErrorAs(t, err, &kcErr)
	})
}

// TestMint_RandReaderError: a failure of the random source surfaces from Mint
// as an error rather than producing a short or predictable key.
//
//nolint:paralleltest // overrides package-level hook vars.
func TestMint_RandReaderError(t *testing.T) {
	withHooks(t, func() {
		randReader = iotest.ErrReader(errors.New("random source unavailable"))

		_, err := Mint()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate data key")
	})
}

// TestKeychainUnavailableError_UnwrapAndError verifies the wrapped cause is
// retrievable via errors.Unwrap/errors.Is and that Error names both the
// wrapper and the underlying cause.
func TestKeychainUnavailableError_UnwrapAndError(t *testing.T) {
	t.Parallel()

	cause := errors.New("dbus unreachable")
	err := &KeychainUnavailableError{Err: cause}

	assert.Equal(t, cause, err.Unwrap())
	require.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "OS keychain unavailable")
	assert.Contains(t, err.Error(), "dbus unreachable")
}
