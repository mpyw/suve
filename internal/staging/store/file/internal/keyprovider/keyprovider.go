// Package keyprovider resolves the AES-256 data key used to encrypt the
// working staging state files (param.json/secret.json).
//
// The resolution follows a fixed fallback chain:
//
//  1. SUVE_STAGING_KEY env var, if set: base64-standard of exactly 32 bytes.
//     If set but invalid, a clear error is returned (no silent fall-through).
//  2. Otherwise the OS keychain (get-or-create): fetch the stored key, or
//     generate 32 random bytes, store them, and use them.
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
// intentional omission (the stash flow still uses a passphrase).
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
// The returned key is 32 bytes when plaintext is false. When plaintext is
// true, key is nil and the caller should operate without encryption (and warn
// the user once). err is non-nil for an invalid SUVE_STAGING_KEY value or for a
// hard keychain failure (as a *KeychainUnavailableError).
func Resolve() (key []byte, plaintext bool, err error) {
	// 1. Environment variable override.
	if raw, ok := lookupEnvFunc(EnvStagingKey); ok {
		envKey, decErr := decodeEnvKey(raw)
		if decErr != nil {
			return nil, false, decErr
		}

		return envKey, false, nil
	}

	// 2. OS keychain get-or-create.
	k, kcErr := getOrCreateKeychainKey()

	switch {
	case kcErr == nil:
		return k, false, nil
	case errors.Is(kcErr, keyring.ErrUnsupportedPlatform):
		// 3. No keyring backend on this platform: documented plaintext fallback.
		return nil, true, nil
	default:
		// Hard keychain failure: surfaced, never silently downgraded.
		return nil, false, &KeychainUnavailableError{Err: kcErr}
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

// getOrCreateKeychainKey fetches the stored key from the OS keychain, or
// generates, stores, and returns a new 32-byte key if none exists.
func getOrCreateKeychainKey() ([]byte, error) {
	stored, err := keyringGetFunc(keychainService, keychainAccount)
	switch {
	case err == nil:
		key, decErr := base64.StdEncoding.DecodeString(stored)
		if decErr != nil {
			return nil, fmt.Errorf("stored keychain key is corrupted: %w", decErr)
		}

		if len(key) != keyLen {
			return nil, fmt.Errorf("stored keychain key has wrong length %d", len(key))
		}

		return key, nil

	case errors.Is(err, keyring.ErrNotFound):
		return generateAndStoreKey()

	default:
		return nil, fmt.Errorf("failed to read key from keychain: %w", err)
	}
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
