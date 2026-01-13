package terminal_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/cli/terminal"
)

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

func TestDefaultWidth(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 50, terminal.DefaultWidth)
}
