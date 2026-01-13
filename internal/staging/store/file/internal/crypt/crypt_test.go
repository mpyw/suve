package crypt

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt(t *testing.T) {
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
			// Encrypt
			encrypted, err := Encrypt(tt.data, tt.passphrase)
			require.NoError(t, err)

			// Verify it's marked as encrypted
			assert.True(t, IsEncrypted(encrypted))

			// Verify encrypted data is different from original (unless empty)
			if len(tt.data) > 0 {
				assert.False(t, bytes.Contains(encrypted, tt.data))
			}

			// Decrypt
			decrypted, err := Decrypt(encrypted, tt.passphrase)
			require.NoError(t, err)

			// Verify decrypted matches original
			// Use bytes.Equal for proper nil/empty comparison
			if len(tt.data) == 0 && len(decrypted) == 0 {
				// Both empty, that's fine
			} else {
				assert.Equal(t, tt.data, decrypted)
			}
		})
	}
}

func TestDecrypt_WrongPassphrase(t *testing.T) {
	data := []byte("secret data")
	passphrase := "correct-password"

	encrypted, err := Encrypt(data, passphrase)
	require.NoError(t, err)

	_, err = Decrypt(encrypted, "wrong-password")
	assert.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestDecrypt_NotEncrypted(t *testing.T) {
	plainData := []byte(`{"version":2,"entries":{}}`)

	_, err := Decrypt(plainData, "any-passphrase")
	assert.ErrorIs(t, err, ErrNotEncrypted)
}

func TestDecrypt_CorruptedData(t *testing.T) {
	data := []byte("secret data")
	passphrase := "secret"

	encrypted, err := Encrypt(data, passphrase)
	require.NoError(t, err)

	// Corrupt the ciphertext (last few bytes)
	encrypted[len(encrypted)-1] ^= 0xff
	encrypted[len(encrypted)-2] ^= 0xff

	_, err = Decrypt(encrypted, passphrase)
	assert.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestDecrypt_TruncatedData(t *testing.T) {
	data := []byte("secret data")
	passphrase := "secret"

	encrypted, err := Encrypt(data, passphrase)
	require.NoError(t, err)

	// Truncate data
	truncated := encrypted[:headerLen+saltLen+nonceLen+5] // Less than minimum

	_, err = Decrypt(truncated, passphrase)
	assert.ErrorIs(t, err, ErrInvalidFormat)
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "encrypted data",
			data:     append([]byte(MagicHeader), Version),
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
			result := IsEncrypted(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncrypt_ProducesDifferentOutput(t *testing.T) {
	data := []byte("same data")
	passphrase := "same passphrase"

	// Encrypt twice
	encrypted1, err := Encrypt(data, passphrase)
	require.NoError(t, err)

	encrypted2, err := Encrypt(data, passphrase)
	require.NoError(t, err)

	// Should produce different ciphertext due to random salt and nonce
	assert.NotEqual(t, encrypted1, encrypted2)

	// But both should decrypt to same data
	decrypted1, err := Decrypt(encrypted1, passphrase)
	require.NoError(t, err)

	decrypted2, err := Decrypt(encrypted2, passphrase)
	require.NoError(t, err)

	assert.Equal(t, decrypted1, decrypted2)
}

func TestDecrypt_UnsupportedVersion(t *testing.T) {
	// Create data with valid header but unsupported version
	data := make([]byte, headerLen+saltLen+nonceLen+20)
	copy(data, []byte(MagicHeader))
	data[len(MagicHeader)] = 99 // Unsupported version

	_, err := Decrypt(data, "any-passphrase")
	assert.ErrorIs(t, err, ErrInvalidFormat)
	assert.Contains(t, err.Error(), "unsupported version 99")
}
