// Package keyprovider resolves the AES-256 data key used to encrypt the
// working staging state files (param.json/secret.json).
//
// The resolution follows a fixed fallback chain:
//
//  1. SUVE_STAGING_KEY env var, if set: base64-standard of exactly 32 bytes.
//     If set but invalid, a clear error is returned (no silent fall-through).
//  2. Otherwise the OS keychain: fetch the stored key. When the keychain is
//     reachable but empty, Resolve reports needsMint instead of generating a
//     key itself, so the caller can refuse to mint a replacement when encrypted
//     working state already exists (a fresh key could not decrypt it). Minting
//     the 32 random bytes and storing them happens in Mint.
//  3. Otherwise (no keyring backend on this platform): plaintext, with the
//     caller expected to warn the user once.
//
// A HARD keychain failure (locked keychain, unreachable dbus, corrupted stored
// key, failed store) is NOT silently downgraded to plaintext: it is returned as
// a *KeychainUnavailableError so the caller can surface the real cause instead
// of letting a nil key later produce a misleading "wrong passphrase" error.
// Only a platform with no keyring backend at all (keyring.ErrUnsupportedPlatform)
// takes the plaintext fallback.
//
// No passphrase prompt is used for the working store: prompting on every
// staging operation would defeat the purpose of the keychain. This is an
// intentional omission (the export/import flow still uses a passphrase).
package keyprovider

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	// EnvStagingKey is the environment variable holding a base64-standard
	// encoded 32-byte key that overrides the keychain.
	EnvStagingKey = "SUVE_STAGING_KEY"

	// keychainService is the OS keychain service name.
	keychainService = "suve"
	// keychainAccount is the OS keychain account/user name for the data key.
	keychainAccount = "staging-data-key"

	// keyLen is the required AES-256 key length in bytes.
	keyLen = 32
)

// Hooks for testing - these allow tests to override environment and keychain
// access without touching the real environment or OS keychain.
//
//nolint:gochecknoglobals // test hooks for dependency injection.
var (
	lookupEnvFunc  = os.LookupEnv
	keyringGetFunc = keyring.Get
	keyringSetFunc = keyring.Set
	randReader     = rand.Reader
)

// ErrKeychainKeyNotFound signals that the OS keychain is reachable but holds no
// stored staging data key. It is the cause reported when a lost keychain entry
// is detected alongside encrypted working state.
var ErrKeychainKeyNotFound = errors.New(
	"no staging data key found in the OS keychain (it may have been deleted or reset)")

// KeychainUnavailableError wraps a hard OS-keychain failure — a locked
// keychain, an unreachable dbus, a corrupted stored key, or a failed store.
//
// It is distinct from a platform with no keyring backend at all (which yields
// the documented plaintext fallback) and from a bad SUVE_STAGING_KEY (a plain
// configuration error). Callers use errors.As to decide whether a plaintext
// fallback is still acceptable (no encrypted state yet) or the error must be
// surfaced (encrypted state exists).
type KeychainUnavailableError struct{ Err error }

func (e *KeychainUnavailableError) Error() string {
	return "OS keychain unavailable: " + e.Err.Error()
}

func (e *KeychainUnavailableError) Unwrap() error { return e.Err }

// Resolve returns the data key for the working staging store.
//
// The returned key is 32 bytes when plaintext and needsMint are both false.
// When plaintext is true, key is nil and the caller should operate without
// encryption (and warn the user once). When needsMint is true, the keychain is
// reachable but has no stored key yet: key is nil and the caller must decide
// whether to mint one (safe only when no encrypted working state exists) by
// calling Mint. err is non-nil for an invalid SUVE_STAGING_KEY value or for a
// hard keychain failure (as a *KeychainUnavailableError).
func Resolve() (key []byte, plaintext, needsMint bool, err error) {
	// 1. Environment variable override.
	if raw, ok := lookupEnvFunc(EnvStagingKey); ok {
		envKey, decErr := decodeEnvKey(raw)
		if decErr != nil {
			return nil, false, false, decErr
		}

		return envKey, false, false, nil
	}

	// 2. OS keychain lookup (minting is deferred to Mint so the caller can gate
	// it on the absence of encrypted state).
	k, found, kcErr := getKeychainKey()

	switch {
	case kcErr == nil && found:
		return k, false, false, nil
	case kcErr == nil && !found:
		// Keychain reachable but empty: genuine first run OR the entry was lost
		// while state persists. The caller gates minting accordingly.
		return nil, false, true, nil
	case errors.Is(kcErr, keyring.ErrUnsupportedPlatform):
		// 3. No keyring backend on this platform: documented plaintext fallback.
		return nil, true, false, nil
	default:
		// Hard keychain failure: surfaced, never silently downgraded.
		return nil, false, false, &KeychainUnavailableError{Err: kcErr}
	}
}

// decodeEnvKey decodes and validates the env-var key.
func decodeEnvKey(raw string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be base64-standard encoded: %w", EnvStagingKey, err)
	}

	if len(decoded) != keyLen {
		return nil, fmt.Errorf("%s must decode to exactly %d bytes, got %d", EnvStagingKey, keyLen, len(decoded))
	}

	return decoded, nil
}

// getKeychainKey fetches the stored key from the OS keychain. found is false
// with a nil error when the keychain is reachable but holds no key yet
// (keyring.ErrNotFound); minting is deferred to Mint.
func getKeychainKey() (key []byte, found bool, err error) {
	stored, err := keyringGetFunc(keychainService, keychainAccount)
	switch {
	case err == nil:
		decoded, decErr := base64.StdEncoding.DecodeString(stored)
		if decErr != nil {
			return nil, false, fmt.Errorf("stored keychain key is corrupted: %w", decErr)
		}

		if len(decoded) != keyLen {
			return nil, false, fmt.Errorf("stored keychain key has wrong length %d", len(decoded))
		}

		return decoded, true, nil

	case errors.Is(err, keyring.ErrNotFound):
		return nil, false, nil

	default:
		return nil, false, fmt.Errorf("failed to read key from keychain: %w", err)
	}
}

// Mint generates a new random 32-byte data key, stores it in the OS keychain,
// and returns it. Callers must confirm no encrypted working state exists before
// minting: a fresh key cannot decrypt state written with a previous key.
func Mint() ([]byte, error) {
	return generateAndStoreKey()
}

// generateAndStoreKey creates a new random 32-byte key and stores it in the
// OS keychain.
func generateAndStoreKey() ([]byte, error) {
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(randReader, key); err != nil {
		return nil, fmt.Errorf("failed to generate data key: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(key)
	if err := keyringSetFunc(keychainService, keychainAccount, encoded); err != nil {
		return nil, fmt.Errorf("failed to store key in keychain: %w", err)
	}

	return key, nil
}
