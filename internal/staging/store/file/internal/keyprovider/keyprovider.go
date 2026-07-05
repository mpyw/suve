// Package keyprovider resolves the AES-256 data key used to encrypt the
// working staging state files (param.json/secret.json).
//
// The resolution follows a fixed fallback chain:
//
//  1. SUVE_STAGING_KEY env var, if set: base64-standard of exactly 32 bytes.
//     If set but invalid, a clear error is returned (no silent fall-through).
//  2. Otherwise the OS keychain (get-or-create): fetch the stored key, or
//     generate 32 random bytes, store them, and use them.
//  3. Otherwise (keychain unavailable/errors and no env var): plaintext, with
//     the caller expected to warn the user once.
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

// Resolve returns the data key for the working staging store.
//
// The returned key is 32 bytes when plaintext is false. When plaintext is
// true, key is nil and the caller should operate without encryption (and warn
// the user once). err is non-nil only for unrecoverable configuration errors
// (currently: an invalid SUVE_STAGING_KEY value).
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
	if kcErr == nil {
		return k, false, nil
	}

	// 3. Plaintext fallback.
	return nil, true, nil
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
