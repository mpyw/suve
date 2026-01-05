package confirm_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/confirm"
)

func TestPrompter_Confirm(t *testing.T) {
	t.Parallel()

	t.Run("skip confirm", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader(""),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.Confirm("test message", true)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("confirm with y", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("y\n"),
			Stdout: io.Discard,
			Stderr: &stderr,
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.True(t, result)
		assert.Contains(t, stderr.String(), "test message")
		assert.Contains(t, stderr.String(), "[y/N]")
	})

	t.Run("confirm with identity displays target info", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:     strings.NewReader("y\n"),
			Stdout:    io.Discard,
			Stderr:    &stderr,
			AccountID: "123456789012",
			Region:    "ap-northeast-1",
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.True(t, result)
		assert.Contains(t, stderr.String(), "Target: 123456789012 / ap-northeast-1")
	})

	t.Run("confirm without identity does not display target info", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("y\n"),
			Stdout: io.Discard,
			Stderr: &stderr,
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.True(t, result)
		assert.NotContains(t, stderr.String(), "Target:")
	})

	t.Run("confirm with yes", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("yes\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("confirm with YES (case insensitive)", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("YES\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("decline with n", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("n\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("decline with no", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("no\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("decline with empty (default no)", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("decline with random input", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("maybe\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("read error", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  &errorReader{},
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		_, err := p.Confirm("test message", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read response")
	})
}

func TestPrompter_ConfirmAction(t *testing.T) {
	t.Parallel()

	t.Run("formats message correctly", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("y\n"),
			Stdout: io.Discard,
			Stderr: &stderr,
		}

		result, err := p.ConfirmAction("Delete", "/my/param", false)
		require.NoError(t, err)
		assert.True(t, result)
		assert.Contains(t, stderr.String(), "Delete /my/param?")
	})

	t.Run("skip confirm", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader(""),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.ConfirmAction("Delete", "/my/param", true)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("confirm action with identity displays target info", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:     strings.NewReader("y\n"),
			Stdout:    io.Discard,
			Stderr:    &stderr,
			AccountID: "123456789012",
			Region:    "ap-northeast-1",
		}

		result, err := p.ConfirmAction("Update", "/my/param", false)
		require.NoError(t, err)
		assert.True(t, result)
		assert.Contains(t, stderr.String(), "Target: 123456789012 / ap-northeast-1")
		assert.Contains(t, stderr.String(), "Update /my/param?")
	})
}

func TestPrompter_PartialIdentity(t *testing.T) {
	t.Parallel()

	t.Run("account only does not display target info", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:     strings.NewReader("y\n"),
			Stdout:    io.Discard,
			Stderr:    &stderr,
			AccountID: "123456789012",
		}

		_, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.NotContains(t, stderr.String(), "Target:")
	})

	t.Run("region only does not display target info", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("y\n"),
			Stdout: io.Discard,
			Stderr: &stderr,
			Region: "ap-northeast-1",
		}

		_, err := p.Confirm("test message", false)
		require.NoError(t, err)
		assert.NotContains(t, stderr.String(), "Target:")
	})
}

func TestPrompter_ConfirmDelete(t *testing.T) {
	t.Parallel()

	t.Run("skip confirm", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader(""),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.ConfirmDelete("/my/param", true)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("confirm with warning", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("y\n"),
			Stdout: io.Discard,
			Stderr: &stderr,
		}

		result, err := p.ConfirmDelete("/my/param", false)
		require.NoError(t, err)
		assert.True(t, result)
		assert.Contains(t, stderr.String(), "permanently delete")
		assert.Contains(t, stderr.String(), "/my/param")
		assert.Contains(t, stderr.String(), "Continue?")
	})

	t.Run("confirm delete with identity displays target info", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		p := &confirm.Prompter{
			Stdin:     strings.NewReader("y\n"),
			Stdout:    io.Discard,
			Stderr:    &stderr,
			AccountID: "123456789012",
			Region:    "ap-northeast-1",
		}

		result, err := p.ConfirmDelete("/my/param", false)
		require.NoError(t, err)
		assert.True(t, result)
		assert.Contains(t, stderr.String(), "Target: 123456789012 / ap-northeast-1")
		assert.Contains(t, stderr.String(), "permanently delete")
	})

	t.Run("decline delete", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  strings.NewReader("n\n"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		result, err := p.ConfirmDelete("/my/param", false)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("read error", func(t *testing.T) {
		t.Parallel()
		p := &confirm.Prompter{
			Stdin:  &errorReader{},
			Stdout: io.Discard,
			Stderr: io.Discard,
		}

		_, err := p.ConfirmDelete("/my/param", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read response")
	})
}

// errorReader is a reader that always returns an error.
type errorReader struct{}

func (e *errorReader) Read(_ []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}
