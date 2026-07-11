package internal_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
)

func TestResolveValue_FromStdin(t *testing.T) {
	t.Parallel()

	t.Run("reads stdin and trims a single trailing newline", func(t *testing.T) {
		t.Parallel()

		value, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			FromStdin: true,
			Stdin:     strings.NewReader("sk-12345\n"),
		})
		require.NoError(t, err)
		assert.True(t, proceed)
		assert.Equal(t, "sk-12345", value)
	})

	t.Run("preserves interior newlines and CRLF trailing", func(t *testing.T) {
		t.Parallel()

		value, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			FromStdin: true,
			Stdin:     strings.NewReader("{\n  \"a\": 1\n}\r\n"),
		})
		require.NoError(t, err)
		assert.True(t, proceed)
		assert.Equal(t, "{\n  \"a\": 1\n}", value)
	})

	t.Run("empty stdin still proceeds with an empty value", func(t *testing.T) {
		t.Parallel()

		value, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			FromStdin: true,
			Stdin:     strings.NewReader(""),
		})
		require.NoError(t, err)
		assert.True(t, proceed)
		assert.Empty(t, value)
	})

	t.Run("combining a positional value with --value-stdin is an error", func(t *testing.T) {
		t.Parallel()

		_, _, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			FromStdin: true,
			HasArg:    true,
			Arg:       "positional",
			Stdin:     strings.NewReader("from-stdin"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot combine a positional value with --value-stdin")
	})

	t.Run("errors before reading stdin when a confirmation prompt is required", func(t *testing.T) {
		t.Parallel()

		reader := strings.NewReader("sk-12345\n")

		_, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			FromStdin:       true,
			ConfirmRequired: true,
			Stdin:           reader,
		})
		require.ErrorIs(t, err, cliinternal.ErrValueStdinNeedsYes)
		assert.False(t, proceed)
		// Stdin must be left untouched so the value is never silently consumed.
		rest, rerr := io.ReadAll(reader)
		require.NoError(t, rerr)
		assert.Equal(t, "sk-12345\n", string(rest))
	})

	t.Run("reads stdin when confirmation is skipped via --yes", func(t *testing.T) {
		t.Parallel()

		value, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			FromStdin:       true,
			ConfirmRequired: false,
			Stdin:           strings.NewReader("sk-12345\n"),
		})
		require.NoError(t, err)
		assert.True(t, proceed)
		assert.Equal(t, "sk-12345", value)
	})
}

func TestResolveValue_FromArg(t *testing.T) {
	t.Parallel()

	value, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
		HasArg: true,
		Arg:    "hunter2",
	})
	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Equal(t, "hunter2", value)
}

func TestResolveValue_EditorFallback(t *testing.T) {
	t.Parallel()

	t.Run("opens the editor with an empty buffer and uses the result", func(t *testing.T) {
		t.Parallel()

		var gotInitial string

		value, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			OpenEditor: func(_ context.Context, content string) (string, error) {
				gotInitial = content

				return "edited-secret", nil
			},
		})
		require.NoError(t, err)
		assert.True(t, proceed)
		assert.Equal(t, "edited-secret", value)
		assert.Empty(t, gotInitial, "the editor should open on an empty buffer")
	})

	t.Run("an empty editor result cancels (proceed=false)", func(t *testing.T) {
		t.Parallel()

		value, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			OpenEditor: func(_ context.Context, _ string) (string, error) {
				return "", nil
			},
		})
		require.NoError(t, err)
		assert.False(t, proceed)
		assert.Empty(t, value)
	})

	t.Run("non-interactive stdin without a value fails instead of hanging on the editor", func(t *testing.T) {
		t.Parallel()

		// No OpenEditor override: the real editor.Open would be selected, but a
		// non-TTY stdin (here strings.Reader) must short-circuit to an error
		// rather than block waiting on an interactive editor.
		_, proceed, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			Stdin: strings.NewReader(""),
		})
		require.Error(t, err)
		assert.False(t, proceed)
		assert.ErrorIs(t, err, cliinternal.ErrValueRequired)
	})

	t.Run("editor errors are surfaced", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("editor exploded")

		_, _, err := cliinternal.ResolveValue(t.Context(), cliinternal.ValueSource{
			OpenEditor: func(_ context.Context, _ string) (string, error) {
				return "", sentinel
			},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
	})
}
