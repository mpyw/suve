package terminal

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockFdWriter implements Fder for testing.
type mockFdWriter struct {
	buf bytes.Buffer
	fd  uintptr
}

func (m *mockFdWriter) Write(p []byte) (n int, err error) {
	return m.buf.Write(p)
}

func (m *mockFdWriter) Fd() uintptr {
	return m.fd
}

//nolint:paralleltest // Test modifies package globals (IsTTY, GetSize)
func TestGetWidthFromWriter_TTY(t *testing.T) {
	origIsTTY := IsTTY
	origGetSize := GetSize

	defer func() {
		IsTTY = origIsTTY
		GetSize = origGetSize
	}()

	IsTTY = func(_ uintptr) bool { return true }
	GetSize = func(_ int) (width, height int, err error) {
		return 120, 40, nil
	}

	w := &mockFdWriter{fd: 1}
	width := GetWidthFromWriter(w)
	assert.Equal(t, 120, width)
}

//nolint:paralleltest // Test modifies package globals (IsTTY)
func TestGetWidthFromWriter_NonTTY(t *testing.T) {
	origIsTTY := IsTTY

	defer func() { IsTTY = origIsTTY }()

	IsTTY = func(_ uintptr) bool { return false }

	w := &mockFdWriter{fd: 1}
	width := GetWidthFromWriter(w)
	assert.Equal(t, DefaultWidth, width)
}

//nolint:paralleltest // Test modifies package globals (IsTTY, GetSize)
func TestGetWidthFromWriter_GetSizeError(t *testing.T) {
	origIsTTY := IsTTY
	origGetSize := GetSize

	defer func() {
		IsTTY = origIsTTY
		GetSize = origGetSize
	}()

	IsTTY = func(_ uintptr) bool { return true }
	GetSize = func(_ int) (width, height int, err error) {
		return 0, 0, assert.AnError
	}

	w := &mockFdWriter{fd: 1}
	width := GetWidthFromWriter(w)
	assert.Equal(t, DefaultWidth, width)
}

//nolint:paralleltest // Test modifies package globals (IsTTY, GetSize)
func TestGetWidthFromWriter_ZeroWidth(t *testing.T) {
	origIsTTY := IsTTY
	origGetSize := GetSize

	defer func() {
		IsTTY = origIsTTY
		GetSize = origGetSize
	}()

	IsTTY = func(_ uintptr) bool { return true }
	GetSize = func(_ int) (width, height int, err error) {
		return 0, 40, nil
	}

	w := &mockFdWriter{fd: 1}
	width := GetWidthFromWriter(w)
	assert.Equal(t, DefaultWidth, width)
}

//nolint:paralleltest // Test modifies package globals (IsTTY)
func TestIsTerminalWriter_TTY(t *testing.T) {
	origIsTTY := IsTTY

	defer func() { IsTTY = origIsTTY }()

	IsTTY = func(_ uintptr) bool { return true }

	w := &mockFdWriter{fd: 1}
	result := IsTerminalWriter(w)
	assert.True(t, result)
}

//nolint:paralleltest // Test modifies package globals (IsTTY)
func TestIsTerminalWriter_NonTTY(t *testing.T) {
	origIsTTY := IsTTY

	defer func() { IsTTY = origIsTTY }()

	IsTTY = func(_ uintptr) bool { return false }

	w := &mockFdWriter{fd: 1}
	result := IsTerminalWriter(w)
	assert.False(t, result)
}
