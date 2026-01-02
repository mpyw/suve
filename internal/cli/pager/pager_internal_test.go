package pager

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockFdWriter is a writer that implements fder interface for testing TTY code path.
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

func (m *mockFdWriter) String() string {
	return m.buf.String()
}

func TestFitsInTerminal_InvalidFd(t *testing.T) {
	t.Parallel()

	// Invalid fd should return false
	result := fitsInTerminal(-1, "test content\n")
	assert.False(t, result)
}

func TestFitsInTerminal_ContentFits(t *testing.T) {
	// Not parallel because we override getTermSize
	original := getTermSize
	defer func() { getTermSize = original }()

	// Mock terminal height of 10 lines
	getTermSize = func(fd int) (width, height int, err error) {
		return 80, 10, nil
	}

	// 5 lines should fit in 10-line terminal
	result := fitsInTerminal(0, "line1\nline2\nline3\nline4\nline5\n")
	assert.True(t, result)
}

func TestFitsInTerminal_ContentDoesNotFit(t *testing.T) {
	// Not parallel because we override getTermSize
	original := getTermSize
	defer func() { getTermSize = original }()

	// Mock terminal height of 5 lines
	getTermSize = func(fd int) (width, height int, err error) {
		return 80, 5, nil
	}

	// 10 lines should NOT fit in 5-line terminal
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	result := fitsInTerminal(0, content)
	assert.False(t, result)
}

func TestFitsInTerminal_ContentWithoutTrailingNewline(t *testing.T) {
	// Not parallel because we override getTermSize
	original := getTermSize
	defer func() { getTermSize = original }()

	// Mock terminal height of 10 lines
	getTermSize = func(fd int) (width, height int, err error) {
		return 80, 10, nil
	}

	// 2 lines without trailing newline should fit
	result := fitsInTerminal(0, "line1\nline2")
	assert.True(t, result)
}

func TestFitsInTerminal_ExactlyFits(t *testing.T) {
	// Not parallel because we override getTermSize
	original := getTermSize
	defer func() { getTermSize = original }()

	// Mock terminal height of 5 lines
	getTermSize = func(fd int) (width, height int, err error) {
		return 80, 5, nil
	}

	// 5 lines should NOT fit (we need lines < height, not <=, to leave margin)
	result := fitsInTerminal(0, "line1\nline2\nline3\nline4\nline5\n")
	assert.False(t, result)
}

func TestFitsInTerminal_ZeroHeight(t *testing.T) {
	// Not parallel because we override getTermSize
	original := getTermSize
	defer func() { getTermSize = original }()

	// Mock zero height terminal
	getTermSize = func(fd int) (width, height int, err error) {
		return 80, 0, nil
	}

	result := fitsInTerminal(0, "line1\n")
	assert.False(t, result)
}

func TestFitsInTerminal_EmptyContent(t *testing.T) {
	// Not parallel because we override getTermSize
	original := getTermSize
	defer func() { getTermSize = original }()

	// Mock terminal height of 10 lines
	getTermSize = func(fd int) (width, height int, err error) {
		return 80, 10, nil
	}

	// Empty content should fit
	result := fitsInTerminal(0, "")
	assert.True(t, result)
}

func TestWithPagerWriter_TTY_EmptyOutput(t *testing.T) {
	// Not parallel because we override globals
	origIsTTY := isTTY
	origGetTermSize := getTermSize
	defer func() {
		isTTY = origIsTTY
		getTermSize = origGetTermSize
	}()

	isTTY = func(fd uintptr) bool { return true }
	getTermSize = func(fd int) (width, height int, err error) {
		return 80, 10, nil
	}

	w := &mockFdWriter{fd: 1}
	err := WithPagerWriter(w, false, func(w io.Writer) error {
		// Write nothing
		return nil
	})

	assert.NoError(t, err)
	assert.Empty(t, w.String())
}

func TestWithPagerWriter_TTY_FitsInTerminal(t *testing.T) {
	// Not parallel because we override globals
	origIsTTY := isTTY
	origGetTermSize := getTermSize
	defer func() {
		isTTY = origIsTTY
		getTermSize = origGetTermSize
	}()

	isTTY = func(fd uintptr) bool { return true }
	getTermSize = func(fd int) (width, height int, err error) {
		return 80, 10, nil
	}

	w := &mockFdWriter{fd: 1}
	err := WithPagerWriter(w, false, func(w io.Writer) error {
		_, err := w.Write([]byte("line1\nline2\nline3\n"))
		return err
	})

	assert.NoError(t, err)
	assert.Equal(t, "line1\nline2\nline3\n", w.String())
}

func TestWithPagerWriter_TTY_ErrorFromFn(t *testing.T) {
	// Not parallel because we override globals
	origIsTTY := isTTY
	defer func() { isTTY = origIsTTY }()

	isTTY = func(fd uintptr) bool { return true }

	w := &mockFdWriter{fd: 1}
	expectedErr := errors.New("test error")
	err := WithPagerWriter(w, false, func(w io.Writer) error {
		return expectedErr
	})

	assert.ErrorIs(t, err, expectedErr)
}
