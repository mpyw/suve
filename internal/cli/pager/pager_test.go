package pager_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/cli/terminal"
)

func TestWithPagerWriter_NoPagerTrue(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := pager.WithPagerWriter(&buf, true, func(w io.Writer) error {
		_, err := w.Write([]byte("test output"))

		return err
	})

	require.NoError(t, err)
	assert.Equal(t, "test output", buf.String())
}

func TestWithPagerWriter_NoPagerFalse_NonTTY(t *testing.T) {
	t.Parallel()

	// When stdout is not a TTY (like in tests), output goes directly to stdout
	var buf bytes.Buffer

	err := pager.WithPagerWriter(&buf, false, func(w io.Writer) error {
		_, err := w.Write([]byte("test output"))

		return err
	})

	require.NoError(t, err)
	assert.Equal(t, "test output", buf.String())
}

func TestWithPagerWriter_ErrorPropagation(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("test error")

	var buf bytes.Buffer

	err := pager.WithPagerWriter(&buf, true, func(_ io.Writer) error {
		return expectedErr
	})

	assert.ErrorIs(t, err, expectedErr)
}

func TestWithPagerWriter_EmptyOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := pager.WithPagerWriter(&buf, true, func(_ io.Writer) error {
		// Write nothing
		return nil
	})

	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestWithPagerWriter_WithFdNonTTY(t *testing.T) {
	t.Parallel()

	// Open /dev/null which has Fd() but is not a TTY
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skip("cannot open /dev/null")
	}

	defer func() { _ = devNull.Close() }()

	var output []byte
	// Since /dev/null is not a TTY, the output should go directly to it
	err = pager.WithPagerWriter(devNull, false, func(w io.Writer) error {
		output = []byte("test output")
		_, err := w.Write(output)

		return err
	})

	require.NoError(t, err)
	assert.Equal(t, "test output", string(output))
}

type fakeTTYWriter struct {
	bytes.Buffer
}

func (f *fakeTTYWriter) Fd() uintptr { return 1 }

// TestWithPagerWriter_WidthSeesRealTerminalUnderPager guards #346: when output
// is buffered for paging on a real TTY, terminal width detection must see the
// real terminal, not the width-less buffer (which fell back to DefaultWidth and
// truncated log --oneline to ~20 chars). Mocks package-global terminal vars, so
// it is not parallel.
//
//nolint:paralleltest // mutates package-global terminal.IsTTY/GetSize
func TestWithPagerWriter_WidthSeesRealTerminalUnderPager(t *testing.T) {
	origTTY, origSize := terminal.IsTTY, terminal.GetSize

	t.Cleanup(func() { terminal.IsTTY, terminal.GetSize = origTTY, origSize })

	terminal.IsTTY = func(uintptr) bool { return true }
	terminal.GetSize = func(int) (int, int, error) { return 123, 40, nil }

	stdout := &fakeTTYWriter{}

	var gotWidth int

	err := pager.WithPagerWriter(stdout, false, func(w io.Writer) error {
		gotWidth = terminal.GetWidthFromWriter(w)
		_, werr := io.WriteString(w, "short line\n")

		return werr
	})
	require.NoError(t, err)
	assert.Equal(t, 123, gotWidth, "render width must reflect the real terminal, not the buffer default")
}
