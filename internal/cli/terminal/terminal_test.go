package terminal_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/cli/terminal"
)

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

			assert.Equal(t, tt.want, terminal.FdToInt(tt.fd))
		})
	}
}

func TestGetWidthFromWriter_NonFder(t *testing.T) {
	t.Parallel()

	// bytes.Buffer doesn't implement Fder, should return DefaultWidth
	var buf bytes.Buffer

	width := terminal.GetWidthFromWriter(&buf)
	assert.Equal(t, terminal.DefaultWidth, width)
}

func TestIsTerminalWriter_NonFder(t *testing.T) {
	t.Parallel()

	// bytes.Buffer doesn't implement Fder, should return false
	var buf bytes.Buffer

	result := terminal.IsTerminalWriter(&buf)
	assert.False(t, result)
}

func TestIsTerminalReader_NonFder(t *testing.T) {
	t.Parallel()

	// bytes.Buffer doesn't implement Fder, so a piped/buffered stdin is never
	// mistaken for an interactive terminal.
	var buf bytes.Buffer

	result := terminal.IsTerminalReader(&buf)
	assert.False(t, result)
}

func TestDefaultWidth(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 50, terminal.DefaultWidth)
}
