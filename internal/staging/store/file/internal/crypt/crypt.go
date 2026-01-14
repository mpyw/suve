// Package crypt provides passphrase-based encryption for staging files.
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
	randReader    io.Reader                                      = rand.Reader
	newCipherFunc func(key []byte) (cipher.Block, error)         = aes.NewCipher
	newGCMFunc    func(cipher cipher.Block) (cipher.AEAD, error) = cipher.NewGCM
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
	// Version is the current encryption format version.
	Version = byte(1)

	// Argon2 parameters (OWASP recommended for sensitive data).
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 4
	argonKeyLen  = 32 // AES-256

	saltLen  = 32
	nonceLen = 12 // AES-GCM standard nonce size
)

var (
	// ErrInvalidFormat is returned when the data format is invalid.
	ErrInvalidFormat = errors.New("invalid encrypted format")
	// ErrDecryptionFailed is returned when decryption fails (wrong passphrase or corrupted data).
	ErrDecryptionFailed = errors.New("decryption failed: wrong passphrase or corrupted data")
	// ErrNotEncrypted is returned when trying to decrypt unencrypted data.
	ErrNotEncrypted = errors.New("data is not encrypted")
)

// headerLen is the total header length: magic (8) + version (1).
const headerLen = len(MagicHeader) + 1

// gcmAuthTagLen is the minimum GCM authentication tag length in bytes.
const gcmAuthTagLen = 16

// Encrypt encrypts data with the given passphrase using AES-256-GCM with Argon2id key derivation.
// Returns encrypted data in format: magic header + version + salt + nonce + ciphertext.
func Encrypt(data []byte, passphrase string) ([]byte, error) {
	// Generate random salt
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(randReader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Create AES cipher
	block, err := newCipherFunc(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := newGCMFunc(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nil, nonce, data, nil)

	// Build output: header + salt + nonce + ciphertext
	result := make([]byte, 0, headerLen+saltLen+nonceLen+len(ciphertext))
	result = append(result, []byte(MagicHeader)...)
	result = append(result, Version)
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// Decrypt decrypts data with the given passphrase.
// Returns ErrNotEncrypted if data doesn't have encryption header.
// Returns ErrDecryptionFailed if passphrase is wrong or data is corrupted.
func Decrypt(data []byte, passphrase string) ([]byte, error) {
	if !IsEncrypted(data) {
		return nil, ErrNotEncrypted
	}

	minLen := headerLen + saltLen + nonceLen + gcmAuthTagLen
	if len(data) < minLen {
		return nil, ErrInvalidFormat
	}

	// Check version
	version := data[len(MagicHeader)]
	if version != Version {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidFormat, version)
	}

	// Extract components
	offset := headerLen
	salt := data[offset : offset+saltLen]
	offset += saltLen
	nonce := data[offset : offset+nonceLen]
	offset += nonceLen
	ciphertext := data[offset:]

	// Derive key using same parameters
	key := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Create AES cipher
	block, err := newCipherFunc(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := newGCMFunc(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// IsEncrypted checks if data has the encryption magic header.
func IsEncrypted(data []byte) bool {
	if len(data) < headerLen {
		return false
	}

	return string(data[:len(MagicHeader)]) == MagicHeader
}
