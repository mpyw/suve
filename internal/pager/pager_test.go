package pager_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/pager"
)

func TestWithPagerWriter_NoPagerTrue(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := pager.WithPagerWriter(&buf, true, func(w io.Writer) error {
		_, err := w.Write([]byte("test output"))
		return err
	})

	assert.NoError(t, err)
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

	assert.NoError(t, err)
	assert.Equal(t, "test output", buf.String())
}

func TestWithPagerWriter_ErrorPropagation(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("test error")
	var buf bytes.Buffer
	err := pager.WithPagerWriter(&buf, true, func(w io.Writer) error {
		return expectedErr
	})

	assert.ErrorIs(t, err, expectedErr)
}

func TestWithPagerWriter_EmptyOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := pager.WithPagerWriter(&buf, true, func(w io.Writer) error {
		// Write nothing
		return nil
	})

	assert.NoError(t, err)
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

	assert.NoError(t, err)
	assert.Equal(t, "test output", string(output))
}
