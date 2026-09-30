// These are terminal.go's tests, so they share its namespace.
//declscope:namespace terminal

package terminal

import (
	"bytes"
	"math"
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
	assert.Equal(t, defaultWidth, width)
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
	assert.Equal(t, defaultWidth, width)
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
	assert.Equal(t, defaultWidth, width)
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

// mockFdReader implements Fder for reader tests.
type mockFdReader struct {
	buf bytes.Buffer
	fd  uintptr
}

func (m *mockFdReader) Read(p []byte) (n int, err error) {
	return m.buf.Read(p)
}

func (m *mockFdReader) Fd() uintptr {
	return m.fd
}

//nolint:paralleltest // Test modifies package globals (IsTTY)
func TestIsTerminalReader_TTY(t *testing.T) {
	origIsTTY := IsTTY

	defer func() { IsTTY = origIsTTY }()

	IsTTY = func(_ uintptr) bool { return true }

	r := &mockFdReader{fd: 0}
	assert.True(t, IsTerminalReader(r))
}

//nolint:paralleltest // Test modifies package globals (IsTTY)
func TestIsTerminalReader_NonTTY(t *testing.T) {
	origIsTTY := IsTTY

	defer func() { IsTTY = origIsTTY }()

	IsTTY = func(_ uintptr) bool { return false }

	r := &mockFdReader{fd: 0}
	assert.False(t, IsTerminalReader(r))
}

func TestFdToInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fd   uintptr
		want int
	}{
		{name: "typical fd", fd: 3, want: 3},
		{name: "zero", fd: 0, want: 0},
		{name: "max int", fd: uintptr(math.MaxInt), want: math.MaxInt},
		{name: "overflow returns -1", fd: uintptr(math.MaxUint), want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, FdToInt(tt.fd))
		})
	}
}

func TestGetWidthFromWriter_NonFder(t *testing.T) {
	t.Parallel()

	// bytes.Buffer doesn't implement Fder, should return defaultWidth
	var buf bytes.Buffer

	width := GetWidthFromWriter(&buf)
	assert.Equal(t, defaultWidth, width)
}

func TestIsTerminalWriter_NonFder(t *testing.T) {
	t.Parallel()

	// bytes.Buffer doesn't implement Fder, should return false
	var buf bytes.Buffer

	result := IsTerminalWriter(&buf)
	assert.False(t, result)
}

func TestIsTerminalReader_NonFder(t *testing.T) {
	t.Parallel()

	// bytes.Buffer doesn't implement Fder, so a piped/buffered stdin is never
	// mistaken for an interactive terminal.
	var buf bytes.Buffer

	result := IsTerminalReader(&buf)
	assert.False(t, result)
}

func TestDefaultWidth(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 50, defaultWidth)
}
