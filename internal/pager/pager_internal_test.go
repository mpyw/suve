package pager

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
