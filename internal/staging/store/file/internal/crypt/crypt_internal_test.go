package crypt

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorReader is an io.Reader that returns an error after reading n bytes
type errorReader struct {
	bytesToRead int
	bytesRead   int
	err         error
}

func (r *errorReader) Read(p []byte) (n int, err error) {
	if r.bytesRead >= r.bytesToRead {
		return 0, r.err
	}
	toRead := len(p)
	if r.bytesRead+toRead > r.bytesToRead {
		toRead = r.bytesToRead - r.bytesRead
	}
	// Fill with zeros
	for i := range toRead {
		p[i] = 0
	}
	r.bytesRead += toRead
	if r.bytesRead >= r.bytesToRead {
		return toRead, r.err
	}
	return toRead, nil
}

func TestEncrypt_SaltGenerationError(t *testing.T) {
	// Save original and restore after test
	original := randReader
	defer func() { randReader = original }()

	// Inject an error reader that fails immediately
	randReader = &errorReader{
		bytesToRead: 0,
		err:         errors.New("random source unavailable"),
	}

	_, err := Encrypt([]byte("test data"), "passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate salt")
}

func TestEncrypt_NonceGenerationError(t *testing.T) {
	// Save original and restore after test
	original := randReader
	defer func() { randReader = original }()

	// Inject an error reader that succeeds for salt (32 bytes) but fails for nonce
	randReader = &errorReader{
		bytesToRead: saltLen, // Succeed for salt, fail for nonce
		err:         errors.New("random source unavailable"),
	}

	_, err := Encrypt([]byte("test data"), "passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate nonce")
}

func TestEncrypt_SuccessWithMockedRandom(t *testing.T) {
	// Save original and restore after test
	original := randReader
	defer func() { randReader = original }()

	// Use real random reader for this test
	randReader = rand.Reader

	data := []byte("test data")
	encrypted, err := Encrypt(data, "passphrase")
	require.NoError(t, err)
	assert.True(t, IsEncrypted(encrypted))

	// Decrypt and verify
	decrypted, err := Decrypt(encrypted, "passphrase")
	require.NoError(t, err)
	assert.Equal(t, data, decrypted)
}

func TestEncrypt_CipherError(t *testing.T) {
	// Save originals and restore after test
	originalRand := randReader
	originalCipher := newCipherFunc
	defer func() {
		randReader = originalRand
		newCipherFunc = originalCipher
	}()

	// Use real random for salt/nonce
	randReader = rand.Reader

	// Inject cipher error
	newCipherFunc = func(_ []byte) (cipher.Block, error) {
		return nil, errors.New("cipher creation failed")
	}

	_, err := Encrypt([]byte("test data"), "passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create cipher")
}

func TestEncrypt_GCMError(t *testing.T) {
	// Save originals and restore after test
	originalRand := randReader
	originalGCM := newGCMFunc
	defer func() {
		randReader = originalRand
		newGCMFunc = originalGCM
	}()

	// Use real random for salt/nonce
	randReader = rand.Reader

	// Inject GCM error
	newGCMFunc = func(_ cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("GCM creation failed")
	}

	_, err := Encrypt([]byte("test data"), "passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create GCM")
}

func TestDecrypt_CipherError(t *testing.T) {
	// First encrypt some data normally
	data := []byte("test data")
	encrypted, err := Encrypt(data, "passphrase")
	require.NoError(t, err)

	// Save original and restore after test
	originalCipher := newCipherFunc
	defer func() { newCipherFunc = originalCipher }()

	// Inject cipher error
	newCipherFunc = func(_ []byte) (cipher.Block, error) {
		return nil, errors.New("cipher creation failed")
	}

	_, err = Decrypt(encrypted, "passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create cipher")
}

func TestDecrypt_GCMError(t *testing.T) {
	// First encrypt some data normally
	data := []byte("test data")
	encrypted, err := Encrypt(data, "passphrase")
	require.NoError(t, err)

	// Save original and restore after test
	originalGCM := newGCMFunc
	defer func() { newGCMFunc = originalGCM }()

	// Inject GCM error
	newGCMFunc = func(_ cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("GCM creation failed")
	}

	_, err = Decrypt(encrypted, "passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create GCM")
}

func TestSetCipherFuncs_AndReset(t *testing.T) {
	// Test that SetCipherFuncs and ResetCipherFuncs work correctly
	called := false
	customCipher := func(_ []byte) (cipher.Block, error) {
		called = true
		return nil, errors.New("custom error")
	}
	customGCM := func(_ cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("custom GCM error")
	}

	SetCipherFuncs(customCipher, customGCM)
	defer ResetCipherFuncs()

	_, err := Encrypt([]byte("test"), "pass")
	assert.True(t, called)
	require.Error(t, err)

	// Reset and verify normal operation
	ResetCipherFuncs()

	result, err := Encrypt([]byte("test"), "pass")
	require.NoError(t, err)
	assert.True(t, IsEncrypted(result))
}

func TestSetRandReader_AndReset(t *testing.T) {
	// Test that SetRandReader and ResetRandReader work correctly

	// Set a custom reader that always returns an error
	SetRandReader(&errorReader{
		bytesToRead: 0,
		err:         errors.New("custom random error"),
	})
	defer ResetRandReader()

	_, err := Encrypt([]byte("test"), "pass")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate salt")

	// Reset and verify normal operation
	ResetRandReader()

	result, err := Encrypt([]byte("test"), "pass")
	require.NoError(t, err)
	assert.True(t, IsEncrypted(result))
}
