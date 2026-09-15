// These are import.go's in-package tests: importPassphrase and the re-anchor
// resolver are private to the import namespace. The file is named import_test.go
// (not *_internal_test.go) because "import" is a Go keyword that
// //declscope:namespace cannot spell, while the _test.go suffix alone already
// derives the same "import" namespace; the black-box command tests moved to
// import_command_test.go.

package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/staging"
)

// =============================================================================
// importPassphrase
// =============================================================================

//nolint:paralleltest // TTY subtests mock the global terminal.IsTTY
func TestImportPassphrase(t *testing.T) {
	t.Run("--passphrase-stdin reads a line", func(t *testing.T) {
		var pass string

		var err error

		runWithCmd(t, importFlags(), bytes.NewBufferString("pw123\n"), &bytes.Buffer{}, &bytes.Buffer{},
			[]string{"--passphrase-stdin"}, func(cmd *cli.Command) {
				pass, err = importPassphrase(cmd, bufio.NewReader(cmd.Root().Reader))
			})

		require.NoError(t, err)
		assert.Equal(t, "pw123", pass)
	})

	t.Run("--passphrase-stdin surfaces a read error", func(t *testing.T) {
		var err error

		runWithCmd(t, importFlags(), errReader{}, &bytes.Buffer{}, &bytes.Buffer{},
			[]string{"--passphrase-stdin"}, func(cmd *cli.Command) {
				_, err = importPassphrase(cmd, bufio.NewReader(cmd.Root().Reader))
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "read passphrase from stdin")
	})

	t.Run("non-TTY without --passphrase-stdin is refused", func(t *testing.T) {
		var err error

		runWithCmd(t, importFlags(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{},
			nil, func(cmd *cli.Command) {
				_, err = importPassphrase(cmd, bufio.NewReader(cmd.Root().Reader))
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-TTY")
	})

	t.Run("TTY prompt error is wrapped", func(t *testing.T) {
		mockTTY(t)

		var err error

		runWithCmd(t, importFlags(), &fakeTTY{}, &fakeTTY{}, &fakeTTY{},
			nil, func(cmd *cli.Command) {
				_, err = importPassphrase(cmd, bufio.NewReader(cmd.Root().Reader))
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "passphrase")
	})
}

// =============================================================================
// newImportReAnchorResolver
// =============================================================================

func TestNewReAnchorResolver(t *testing.T) {
	t.Parallel()

	t.Run("unknown service errors", func(t *testing.T) {
		t.Parallel()

		resolver := newImportReAnchorResolver(t.Context(), map[staging.Service]importReAnchorSpec{})

		got, err := resolver(staging.ServiceParam, "")
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "no strategy configured")
	})

	t.Run("namespaced provider resolves per namespace without caching", func(t *testing.T) {
		t.Parallel()

		sentinel := &stubFullStrategy{}

		var namespaces []string

		var calls int

		resolver := newImportReAnchorResolver(t.Context(), map[staging.Service]importReAnchorSpec{
			staging.ServiceParam: {
				strategyForNamespace: func(_ context.Context, namespace string) (staging.FullStrategy, error) {
					namespaces = append(namespaces, namespace)
					calls++

					return sentinel, nil
				},
			},
		})

		got, err := resolver(staging.ServiceParam, "dev")
		require.NoError(t, err)
		assert.Same(t, sentinel, got)

		_, err = resolver(staging.ServiceParam, "prod")
		require.NoError(t, err)

		// Each call re-resolves for its namespace: no caching on the namespaced path.
		assert.Equal(t, 2, calls)
		assert.Equal(t, []string{"dev", "prod"}, namespaces)
	})

	t.Run("factory result is cached across calls", func(t *testing.T) {
		t.Parallel()

		var calls int

		var built []*stubFullStrategy

		resolver := newImportReAnchorResolver(t.Context(), map[staging.Service]importReAnchorSpec{
			staging.ServiceParam: {
				factory: func(context.Context) (staging.FullStrategy, error) {
					calls++
					s := &stubFullStrategy{}
					built = append(built, s)

					return s, nil
				},
			},
		})

		first, err := resolver(staging.ServiceParam, "")
		require.NoError(t, err)

		second, err := resolver(staging.ServiceParam, "")
		require.NoError(t, err)

		// The factory ran once; both calls return the same cached strategy.
		assert.Equal(t, 1, calls)
		assert.Len(t, built, 1)
		assert.Same(t, built[0], first)
		assert.Same(t, first, second)
	})

	t.Run("factory error propagates and is not cached", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("factory boom")

		var calls int

		resolver := newImportReAnchorResolver(t.Context(), map[staging.Service]importReAnchorSpec{
			staging.ServiceParam: {
				factory: func(context.Context) (staging.FullStrategy, error) {
					calls++

					return nil, sentinel
				},
			},
		})

		_, err := resolver(staging.ServiceParam, "")
		require.ErrorIs(t, err, sentinel)

		// A failed build is not cached, so a retry runs the factory again.
		_, err = resolver(staging.ServiceParam, "")
		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, 2, calls)
	})
}
