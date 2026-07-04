package crypt_test

import (
	"bytes"
	"encoding/base64"
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
		{ //nolint:gosec // G101: test fixture passphrase, not a real credential.
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

func TestEncryptDecryptWithKey(t *testing.T) {
	t.Parallel()

	key := make([]byte, crypt.RawKeyLen)
	for i := range key {
		key[i] = byte(i)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{name: "simple text", data: []byte("hello world")},
		{name: "empty data", data: []byte{}},
		{name: "JSON data", data: []byte(`{"version":2,"entries":{}}`)},
		{name: "binary data", data: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encrypted, err := crypt.EncryptWithKey(tt.data, key)
			require.NoError(t, err)

			// v2 must still be recognized as encrypted.
			assert.True(t, crypt.IsEncrypted(encrypted))
			// Version byte must be the raw-key version.
			assert.Equal(t, crypt.VersionRawKey, encrypted[len(crypt.MagicHeader)])

			if len(tt.data) > 0 {
				assert.False(t, bytes.Contains(encrypted, tt.data))
			}

			decrypted, err := crypt.DecryptWithKey(encrypted, key)
			require.NoError(t, err)

			if len(tt.data) != 0 || len(decrypted) != 0 {
				assert.Equal(t, tt.data, decrypted)
			}
		})
	}
}

func TestEncryptWithKey_InvalidKeyLength(t *testing.T) {
	t.Parallel()

	_, err := crypt.EncryptWithKey([]byte("data"), make([]byte, 16))
	assert.ErrorIs(t, err, crypt.ErrInvalidKeyLength)
}

func TestDecryptWithKey_InvalidKeyLength(t *testing.T) {
	t.Parallel()

	_, err := crypt.DecryptWithKey([]byte("data"), make([]byte, 16))
	assert.ErrorIs(t, err, crypt.ErrInvalidKeyLength)
}

func TestDecryptWithKey_WrongKey(t *testing.T) {
	t.Parallel()

	key := make([]byte, crypt.RawKeyLen)
	wrongKey := make([]byte, crypt.RawKeyLen)
	wrongKey[0] = 0xff

	encrypted, err := crypt.EncryptWithKey([]byte("secret"), key)
	require.NoError(t, err)

	_, err = crypt.DecryptWithKey(encrypted, wrongKey)
	assert.ErrorIs(t, err, crypt.ErrDecryptionFailed)
}

// TestCrossReject verifies the two formats reject each other's data.
func TestCrossReject(t *testing.T) {
	t.Parallel()

	key := make([]byte, crypt.RawKeyLen)

	t.Run("Decrypt (passphrase) rejects v2 data", func(t *testing.T) {
		t.Parallel()

		v2, err := crypt.EncryptWithKey([]byte("data"), key)
		require.NoError(t, err)

		_, err = crypt.Decrypt(v2, "any-passphrase")
		require.ErrorIs(t, err, crypt.ErrInvalidFormat)
		assert.Contains(t, err.Error(), "raw-key")
	})

	t.Run("DecryptWithKey rejects v1 data", func(t *testing.T) {
		t.Parallel()

		v1, err := crypt.Encrypt([]byte("data"), "passphrase")
		require.NoError(t, err)

		_, err = crypt.DecryptWithKey(v1, key)
		require.ErrorIs(t, err, crypt.ErrInvalidFormat)
		assert.Contains(t, err.Error(), "version 1")
	})
}

func TestDecryptWithKey_NotEncrypted(t *testing.T) {
	t.Parallel()

	_, err := crypt.DecryptWithKey([]byte(`{"plain":true}`), make([]byte, crypt.RawKeyLen))
	assert.ErrorIs(t, err, crypt.ErrNotEncrypted)
}

// TestDecrypt_V1BackwardCompat verifies a v1 blob produced by an earlier build
// still decrypts. This is a golden fixture: the salt/nonce are embedded, so the
// Argon2 parameters must be resolved from the version byte via the params
// table for this to succeed.
func TestDecrypt_V1BackwardCompat(t *testing.T) {
	t.Parallel()

	const goldenV1 = "U1VWRV9FTkMBMV+dH6A0LNn0lG8s9emCVJeLMiC2iSLX+G+ASrVwRHB4n4g+ir2sxHh6dm/WXWFWlY86eisG0WKEolYqnEFBy9EG2OWIQtiNs96Zkas="

	blob, err := base64.StdEncoding.DecodeString(goldenV1)
	require.NoError(t, err)

	// Sanity: it is a v1 blob.
	require.True(t, crypt.IsEncrypted(blob))
	require.Equal(t, crypt.Version, blob[len(crypt.MagicHeader)])

	decrypted, err := crypt.Decrypt(blob, "golden-passphrase")
	require.NoError(t, err)
	assert.JSONEq(t, `{"hello":"world"}`, string(decrypted))
}
