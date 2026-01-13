package security

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBuffer_EmptyData(t *testing.T) {
	buf := NewBuffer(nil)
	assert.True(t, buf.IsEmpty())

	buf2 := NewBuffer([]byte{})
	assert.True(t, buf2.IsEmpty())
}

func TestNewBuffer_WithData(t *testing.T) {
	data := []byte("secret data")
	original := make([]byte, len(data))
	copy(original, data)

	buf := NewBuffer(data)
	assert.False(t, buf.IsEmpty())

	// Original data should be zeroed
	assert.True(t, bytes.Equal(data, make([]byte, len(data))))
}

func TestBuffer_Bytes(t *testing.T) {
	original := []byte("secret data")
	data := make([]byte, len(original))
	copy(data, original)

	buf := NewBuffer(data)

	result, err := buf.Bytes()
	require.NoError(t, err)
	assert.Equal(t, original, result)
}

func TestBuffer_Bytes_Empty(t *testing.T) {
	buf := NewBuffer(nil)

	result, err := buf.Bytes()
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestBuffer_IsEmpty(t *testing.T) {
	emptyBuf := NewBuffer(nil)
	assert.True(t, emptyBuf.IsEmpty())

	dataBuf := NewBuffer([]byte("data"))
	assert.False(t, dataBuf.IsEmpty())
}

func TestBuffer_Destroy(t *testing.T) {
	buf := NewBuffer([]byte("secret"))
	assert.False(t, buf.IsEmpty())

	buf.Destroy()
	assert.True(t, buf.IsEmpty())

	// Destroy should be idempotent
	buf.Destroy()
	assert.True(t, buf.IsEmpty())
}

func TestBuffer_MultipleBytesCall(t *testing.T) {
	original := []byte("secret data")
	data := make([]byte, len(original))
	copy(data, original)

	buf := NewBuffer(data)

	// Multiple calls should return the same data
	result1, err := buf.Bytes()
	require.NoError(t, err)
	assert.Equal(t, original, result1)

	result2, err := buf.Bytes()
	require.NoError(t, err)
	assert.Equal(t, original, result2)
}
