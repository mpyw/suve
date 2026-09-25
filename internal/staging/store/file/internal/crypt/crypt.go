// Package crypt provides encryption for staging files.
//
// Two on-disk formats are supported:
//
//	v1 (passphrase): magic(8) + version(1)=1 + salt(32) + nonce(12) + ciphertext
//	                 key derived from the passphrase via Argon2id.
//	v2 (raw key):    magic(8) + version(1)=2 + nonce(12) + ciphertext
//	                 the supplied 32-byte key is used directly as the AES-256-GCM key.
//	                 Legacy: read-only, no associated data.
//	v3 (raw key):    magic(8) + version(1)=3 + nonce(12) + ciphertext
//	                 like v2, but the header and caller-supplied associated data
//	                 (the working store binds the scope and service) are
//	                 authenticated, so a file moved to another scope or service
//	                 fails to decrypt.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// Hooks for testing - these allow tests to inject errors.
//
//nolint:gochecknoglobals // Testing hooks to inject errors into crypto operations.
var (
	randReader    = rand.Reader
	newCipherFunc = aes.NewCipher
	newGCMFunc    = cipher.NewGCM
)

// SetRandReader sets the random reader for testing purposes.
// This should only be used in tests.
func SetRandReader(r io.Reader) {
	randReader = r
}

// ResetRandReader resets the random reader to the default.
func ResetRandReader() {
	randReader = rand.Reader
}

// SetCipherFuncs sets the cipher creation functions for testing.
// This should only be used in tests.
func SetCipherFuncs(newCipher func(key []byte) (cipher.Block, error), newGCM func(cipher cipher.Block) (cipher.AEAD, error)) {
	newCipherFunc = newCipher
	newGCMFunc = newGCM
}

// ResetCipherFuncs resets the cipher functions to defaults.
func ResetCipherFuncs() {
	newCipherFunc = aes.NewCipher
	newGCMFunc = cipher.NewGCM
}

const (
	// MagicHeader identifies encrypted files.
	MagicHeader = "SUVE_ENC"
	// Version is the passphrase-based (Argon2id) encryption format version.
	Version = byte(1)
	// VersionRawKey is the legacy raw-key encryption format version.
	// It stores no salt and performs no KDF; the supplied 32-byte key is used
	// directly as the AES-256-GCM key. It binds no associated data. It is only
	// read (DecryptWithKey), never written.
	VersionRawKey = byte(2)
	// VersionRawKeyAAD is the raw-key encryption format version that EncryptWithKey
	// writes: the v2 layout, with the header and the caller's associated data
	// authenticated by AES-GCM.
	VersionRawKeyAAD = byte(3)

	saltLen  = 32
	nonceLen = 12 // AES-GCM standard nonce size

	// RawKeyLen is the required length (in bytes) of a raw AES-256 key.
	RawKeyLen = 32
)

// argonParams holds the Argon2id parameters for a passphrase-based format version.
type argonParams struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
}

// kdfParamsByVersion maps a passphrase-format version byte to its Argon2id
// parameters. Decrypt looks up parameters by the version byte read from the
// file, so a future parameter change is expressed as a new version while old
// files continue to decrypt with the parameters they were written with.
//
//nolint:gochecknoglobals // immutable lookup table keyed by format version.
var kdfParamsByVersion = map[byte]argonParams{
	// v1 uses OWASP-recommended parameters for sensitive data.
	//nolint:mnd // Argon2id parameters (OWASP recommended); a change means a new version.
	1: {time: 3, memory: 64 * 1024, threads: 4, keyLen: 32},
}

var (
	// ErrInvalidFormat is returned when the data format is invalid.
	ErrInvalidFormat = errors.New("invalid encrypted format")
	// ErrDecryptionFailed is returned when passphrase decryption fails (wrong
	// passphrase or corrupted data).
	ErrDecryptionFailed = errors.New("decryption failed: wrong passphrase or corrupted data")
	// ErrKeyMismatch is returned when raw-key decryption fails. The working
	// staging area never uses a passphrase, so the likely cause is a data key
	// that differs from the one the file was written with.
	ErrKeyMismatch = errors.New(
		"decryption failed: the staging data key does not match the one this data was written with " +
			"(is SUVE_STAGING_KEY set differently from before, or was the keychain key replaced?), or the data is corrupted",
	)
	// ErrNotEncrypted is returned when trying to decrypt unencrypted data.
	ErrNotEncrypted = errors.New("data is not encrypted")
	// ErrInvalidKeyLength is returned when a raw key is not exactly RawKeyLen bytes.
	ErrInvalidKeyLength = errors.New("invalid key length: must be 32 bytes")
)

// headerLen is the total header length: magic (8) + version (1).
const headerLen = len(MagicHeader) + 1

// gcmAuthTagLen is the minimum GCM authentication tag length in bytes.
const gcmAuthTagLen = 16

// newGCM creates an AES-GCM AEAD from the given key using the (overridable) cipher hooks.
func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := newCipherFunc(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := newGCMFunc(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return gcm, nil
}

// Encrypt encrypts data with the given passphrase using AES-256-GCM with Argon2id key derivation.
// Returns encrypted data in format: magic header + version(1) + salt + nonce + ciphertext.
//
// It is a convenience wrapper around EncryptWithAAD with no associated data.
func Encrypt(data []byte, passphrase string) ([]byte, error) {
	return EncryptWithAAD(data, passphrase, nil)
}

// EncryptWithAAD is Encrypt with additional authenticated data (AAD) bound to
// the ciphertext via AES-GCM. The AAD is authenticated but not encrypted; the
// exact same aad must be supplied to DecryptWithAAD or authentication fails
// with ErrDecryptionFailed. A nil aad is equivalent to Encrypt.
func EncryptWithAAD(data []byte, passphrase string, aad []byte) ([]byte, error) {
	params := kdfParamsByVersion[Version]

	// Generate random salt
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(randReader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(passphrase), salt, params.time, params.memory, params.threads, params.keyLen)

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	// Generate random nonce
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data, binding the AAD (if any) to the ciphertext.
	ciphertext := gcm.Seal(nil, nonce, data, aad)

	// Build output: header + salt + nonce + ciphertext
	result := make([]byte, 0, headerLen+saltLen+nonceLen+len(ciphertext))
	result = append(result, []byte(MagicHeader)...)
	result = append(result, Version)
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// Decrypt decrypts passphrase-based (v1) data with the given passphrase.
// Returns ErrNotEncrypted if data doesn't have the encryption header.
// Returns ErrInvalidFormat for raw-key (v2) or unknown-version data.
// Returns ErrDecryptionFailed if the passphrase is wrong or data is corrupted.
//
// It is a convenience wrapper around DecryptWithAAD with no associated data.
func Decrypt(data []byte, passphrase string) ([]byte, error) {
	return DecryptWithAAD(data, passphrase, nil)
}

// DecryptWithAAD is Decrypt with additional authenticated data (AAD). The aad
// must be byte-identical to the aad passed to EncryptWithAAD; otherwise GCM
// authentication fails and ErrDecryptionFailed is returned. A nil aad is
// equivalent to Decrypt.
func DecryptWithAAD(data []byte, passphrase string, aad []byte) ([]byte, error) {
	if !IsEncrypted(data) {
		return nil, ErrNotEncrypted
	}

	if len(data) < headerLen {
		return nil, ErrInvalidFormat
	}

	version := data[len(MagicHeader)]

	if version == VersionRawKey || version == VersionRawKeyAAD {
		return nil, fmt.Errorf("%w: data uses raw-key format (v%d); a passphrase cannot decrypt it", ErrInvalidFormat, version)
	}

	params, ok := kdfParamsByVersion[version]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidFormat, version)
	}

	minLen := headerLen + saltLen + nonceLen + gcmAuthTagLen
	if len(data) < minLen {
		return nil, ErrInvalidFormat
	}

	// Extract components
	offset := headerLen
	salt := data[offset : offset+saltLen]
	offset += saltLen
	nonce := data[offset : offset+nonceLen]
	offset += nonceLen
	ciphertext := data[offset:]

	// Derive key using the parameters for this version
	key := argon2.IDKey([]byte(passphrase), salt, params.time, params.memory, params.threads, params.keyLen)

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// EncryptWithKey encrypts data using AES-256-GCM with the supplied 32-byte key
// used directly as the key (no KDF). Returns raw-key (v3) format:
// magic header + version(1)=3 + nonce + ciphertext. The header and aad are
// authenticated (not encrypted); DecryptWithKey must be given the same aad.
func EncryptWithKey(data, key, aad []byte) ([]byte, error) {
	if len(key) != RawKeyLen {
		return nil, ErrInvalidKeyLength
	}

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	// Generate random nonce
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	header := append([]byte(MagicHeader), VersionRawKeyAAD)

	ciphertext := gcm.Seal(nil, nonce, data, rawKeyAAD(header, aad))

	// Build output: header + nonce + ciphertext (no salt)
	result := make([]byte, 0, headerLen+nonceLen+len(ciphertext))
	result = append(result, header...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// DecryptWithKey decrypts raw-key data using the supplied 32-byte key. It reads
// the v3 format, authenticating the header and aad (which must match the aad
// given to EncryptWithKey), and the legacy v2 format, which binds no associated
// data (aad is then ignored).
// Returns ErrNotEncrypted if data doesn't have the encryption header.
// Returns ErrInvalidFormat if the data is not a raw-key format.
// Returns ErrKeyMismatch if the key or aad is wrong or data is corrupted.
func DecryptWithKey(data, key, aad []byte) ([]byte, error) {
	if len(key) != RawKeyLen {
		return nil, ErrInvalidKeyLength
	}

	if !IsEncrypted(data) {
		return nil, ErrNotEncrypted
	}

	if len(data) < headerLen {
		return nil, ErrInvalidFormat
	}

	var boundAAD []byte

	switch version := data[len(MagicHeader)]; version {
	case VersionRawKeyAAD:
		boundAAD = rawKeyAAD(data[:headerLen], aad)
	case VersionRawKey:
		// Legacy: written before associated data was bound.
	default:
		return nil, fmt.Errorf("%w: expected raw-key format (v2 or v3), got version %d", ErrInvalidFormat, version)
	}

	minLen := headerLen + nonceLen + gcmAuthTagLen
	if len(data) < minLen {
		return nil, ErrInvalidFormat
	}

	offset := headerLen
	nonce := data[offset : offset+nonceLen]
	offset += nonceLen
	ciphertext := data[offset:]

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, boundAAD)
	if err != nil {
		return nil, ErrKeyMismatch
	}

	return plaintext, nil
}

// rawKeyAAD is the associated data a v3 raw-key ciphertext authenticates: its
// header followed by the caller's aad, so neither the format version nor the
// caller's binding can be swapped.
func rawKeyAAD(header, aad []byte) []byte {
	return append(append(make([]byte, 0, len(header)+len(aad)), header...), aad...)
}

// IsEncrypted checks if data has the encryption magic header.
func IsEncrypted(data []byte) bool {
	if len(data) < headerLen {
		return false
	}

	return string(data[:len(MagicHeader)]) == MagicHeader
}
