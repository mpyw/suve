package crypt_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

func TestEncryptDecrypt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       []byte
		passphrase string
	}{
		{
			name:       "simple text",
			data:       []byte("hello world"),
			passphrase: "secret123",
		},
		{
			name:       "empty data",
			data:       []byte{},
			passphrase: "secret123",
		},
		{
			name:       "JSON data",
			data:       []byte(`{"version":2,"entries":{}}`),
			passphrase: "my-passphrase",
		},
		{
			name:       "unicode passphrase",
			data:       []byte("test data"),
			passphrase: "パスワード123",
		},
		{
			name:       "long passphrase",
			data:       []byte("test data"),
			passphrase: "this is a very long passphrase that exceeds normal length expectations",
		},
		{
			name:       "binary data",
			data:       []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
			passphrase: "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Encrypt
			encrypted, err := crypt.Encrypt(tt.data, tt.passphrase)
			require.NoError(t, err)

			// Verify it's marked as encrypted
			assert.True(t, crypt.IsEncrypted(encrypted))

			// Verify encrypted data is different from original (unless empty)
			if len(tt.data) > 0 {
				assert.False(t, bytes.Contains(encrypted, tt.data))
			}

			// Decrypt
			decrypted, err := crypt.Decrypt(encrypted, tt.passphrase)
			require.NoError(t, err)

			// Verify decrypted matches original
			// Use bytes.Equal for proper nil/empty comparison
			if len(tt.data) != 0 || len(decrypted) != 0 {
				assert.Equal(t, tt.data, decrypted)
			}
		})
	}
}

func TestDecrypt_WrongPassphrase(t *testing.T) {
	t.Parallel()

	data := []byte("secret data")
	passphrase := "correct-password"

	encrypted, err := crypt.Encrypt(data, passphrase)
	require.NoError(t, err)

	_, err = crypt.Decrypt(encrypted, "wrong-password")
	assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
}

func TestDecrypt_NotEncrypted(t *testing.T) {
	t.Parallel()

	plainData := []byte(`{"version":2,"entries":{}}`)

	_, err := crypt.Decrypt(plainData, "any-passphrase")
	assert.ErrorIs(t, err, crypt.ErrNotEncrypted)
}

func TestDecrypt_CorruptedData(t *testing.T) {
	t.Parallel()

	data := []byte("secret data")
	passphrase := "secret"

	encrypted, err := crypt.Encrypt(data, passphrase)
	require.NoError(t, err)

	// Corrupt the ciphertext (last few bytes)
	encrypted[len(encrypted)-1] ^= 0xff
	encrypted[len(encrypted)-2] ^= 0xff

	_, err = crypt.Decrypt(encrypted, passphrase)
	assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
}

func TestDecrypt_TruncatedData(t *testing.T) {
	t.Parallel()

	data := []byte("secret data")
	passphrase := "secret"

	encrypted, err := crypt.Encrypt(data, passphrase)
	require.NoError(t, err)

	// Truncate data - minimum length is header(9) + salt(32) + nonce(12) + authTag(16) = 69
	// Use a length less than this to trigger ErrInvalidFormat
	minLen := len(crypt.MagicHeader) + 1 + 32 + 12 + 16 // header + salt + nonce + tag = 69
	truncated := encrypted[:minLen-10]                  // Less than minimum

	_, err = crypt.Decrypt(truncated, passphrase)
	assert.ErrorIs(t, err, crypt.ErrInvalidFormat)
}

func TestIsEncrypted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "encrypted data",
			data:     append([]byte(crypt.MagicHeader), crypt.Version),
			expected: true,
		},
		{
			name:     "plain JSON",
			data:     []byte(`{"version":2}`),
			expected: false,
		},
		{
			name:     "empty data",
			data:     []byte{},
			expected: false,
		},
		{
			name:     "short data",
			data:     []byte("SUVE"),
			expected: false,
		},
		{
			name:     "similar but different header",
			data:     []byte("SUVE_DEC\x01"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := crypt.IsEncrypted(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncrypt_ProducesDifferentOutput(t *testing.T) {
	t.Parallel()

	data := []byte("same data")
	passphrase := "same passphrase"

	// Encrypt twice
	encrypted1, err := crypt.Encrypt(data, passphrase)
	require.NoError(t, err)

	encrypted2, err := crypt.Encrypt(data, passphrase)
	require.NoError(t, err)

	// Should produce different ciphertext due to random salt and nonce
	assert.NotEqual(t, encrypted1, encrypted2)

	// But both should decrypt to same data
	decrypted1, err := crypt.Decrypt(encrypted1, passphrase)
	require.NoError(t, err)

	decrypted2, err := crypt.Decrypt(encrypted2, passphrase)
	require.NoError(t, err)

	assert.Equal(t, decrypted1, decrypted2)
}

func TestDecrypt_UnsupportedVersion(t *testing.T) {
	t.Parallel()

	// Create data with valid header but unsupported version
	// header(9) + salt(32) + nonce(12) + some data(20) = 73 bytes
	data := make([]byte, len(crypt.MagicHeader)+1+32+12+20)
	copy(data, []byte(crypt.MagicHeader))
	data[len(crypt.MagicHeader)] = 99 // Unsupported version

	_, err := crypt.Decrypt(data, "any-passphrase")
	require.ErrorIs(t, err, crypt.ErrInvalidFormat)
	assert.Contains(t, err.Error(), "unsupported version 99")
}
