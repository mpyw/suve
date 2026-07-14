package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/staging"
)

// =============================================================================
// Test doubles
// =============================================================================

// fakeTTY is a bytes.Buffer that also satisfies terminal.Fder, so
// terminal.IsTerminalWriter / IsTerminalReader classify it as a terminal once
// terminal.IsTTY is mocked. Fd() returns a value that FdToInt maps to an invalid
// descriptor, so a passphrase prompt's term.ReadPassword fails fast (ENOTTY/EBADF)
// instead of touching the real fd 0.
type fakeTTY struct {
	bytes.Buffer
}

func (f *fakeTTY) Fd() uintptr { return ^uintptr(0) }

// errReader always fails, so a stdin read surfaces a non-EOF error.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

// stubFullStrategy is a sentinel staging.FullStrategy. It embeds the interface so
// it satisfies the type without implementing every method; the re-anchor resolver
// only stores and returns it, never invoking a method.
type stubFullStrategy struct {
	staging.FullStrategy
}

// mockTTY forces terminal.IsTTY to report a terminal for the duration of the
// test. It mutates a global, so callers must not run in parallel.
func mockTTY(t *testing.T) {
	t.Helper()

	orig := terminal.IsTTY

	t.Cleanup(func() { terminal.IsTTY = orig })

	terminal.IsTTY = func(uintptr) bool { return true }
}

// runWithCmd wires reader/writer/errWriter onto a root app and parses args into a
// leaf command carrying flags, then invokes fn from inside the leaf's action so
// fn sees fully-parsed flag values and a resolvable cmd.Root().
func runWithCmd(
	t *testing.T,
	flags []cli.Flag,
	reader io.Reader,
	writer, errWriter io.Writer,
	args []string,
	fn func(cmd *cli.Command),
) {
	t.Helper()

	leaf := &cli.Command{
		Name:  "leaf",
		Flags: flags,
		Action: func(_ context.Context, cmd *cli.Command) error {
			fn(cmd)

			return nil
		},
	}
	app := &cli.Command{
		Name:      "suve",
		Reader:    reader,
		Writer:    writer,
		ErrWriter: errWriter,
		Commands:  []*cli.Command{leaf},
	}

	require.NoError(t, app.Run(t.Context(), append([]string{"suve", "leaf"}, args...)))
}

// =============================================================================
// exportPassphrase
// =============================================================================

//nolint:paralleltest // TTY subtests mock the global terminal.IsTTY
func TestExportPassphrase(t *testing.T) {
	t.Run("--passphrase-stdin reads a line", func(t *testing.T) {
		var pass string

		var cancelled bool

		var err error

		runWithCmd(t, exportFlags(), bytes.NewBufferString("pw123\n"), &bytes.Buffer{}, &bytes.Buffer{},
			[]string{"--passphrase-stdin"}, func(cmd *cli.Command) {
				pass, cancelled, err = exportPassphrase(cmd, bufio.NewReader(cmd.Root().Reader))
			})

		require.NoError(t, err)
		assert.False(t, cancelled)
		assert.Equal(t, "pw123", pass)
	})

	t.Run("--passphrase-stdin surfaces a read error", func(t *testing.T) {
		var err error

		runWithCmd(t, exportFlags(), errReader{}, &bytes.Buffer{}, &bytes.Buffer{},
			[]string{"--passphrase-stdin"}, func(cmd *cli.Command) {
				_, _, err = exportPassphrase(cmd, bufio.NewReader(cmd.Root().Reader))
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "read passphrase from stdin")
	})

	t.Run("non-TTY falls back to plaintext with a warning", func(t *testing.T) {
		var pass string

		var cancelled bool

		var err error

		errBuf := &bytes.Buffer{}

		runWithCmd(t, exportFlags(), &bytes.Buffer{}, &bytes.Buffer{}, errBuf,
			nil, func(cmd *cli.Command) {
				pass, cancelled, err = exportPassphrase(cmd, bufio.NewReader(cmd.Root().Reader))
			})

		require.NoError(t, err)
		assert.False(t, cancelled)
		assert.Empty(t, pass)
		assert.Contains(t, errBuf.String(), "plain text")
	})

	t.Run("TTY prompt error is wrapped", func(t *testing.T) {
		mockTTY(t)

		var err error

		runWithCmd(t, exportFlags(), &fakeTTY{}, &fakeTTY{}, &fakeTTY{},
			nil, func(cmd *cli.Command) {
				_, _, err = exportPassphrase(cmd, bufio.NewReader(cmd.Root().Reader))
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "passphrase")
	})
}

// =============================================================================
// confirmExportOverwrite
// =============================================================================

//nolint:paralleltest // TTY subtests mock the global terminal.IsTTY
func TestConfirmExportOverwrite(t *testing.T) {
	t.Run("--yes skips the check", func(t *testing.T) {
		var proceed bool

		var err error

		// The path exists, but --yes short-circuits before any stat.
		existing := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, os.WriteFile(existing, []byte("{}"), 0o600))

		runWithCmd(t, exportFlags(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{},
			[]string{"--yes"}, func(cmd *cli.Command) {
				proceed, err = confirmExportOverwrite(cmd, []string{existing}, bufio.NewReader(cmd.Root().Reader))
			})

		require.NoError(t, err)
		assert.True(t, proceed)
	})

	t.Run("no existing files proceeds", func(t *testing.T) {
		var proceed bool

		var err error

		missing := filepath.Join(t.TempDir(), "param.json")

		runWithCmd(t, exportFlags(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{},
			nil, func(cmd *cli.Command) {
				proceed, err = confirmExportOverwrite(cmd, []string{missing}, bufio.NewReader(cmd.Root().Reader))
			})

		require.NoError(t, err)
		assert.True(t, proceed)
	})

	t.Run("non-TTY with existing files is refused", func(t *testing.T) {
		var err error

		existing := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, os.WriteFile(existing, []byte("{}"), 0o600))

		runWithCmd(t, exportFlags(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{},
			nil, func(cmd *cli.Command) {
				_, err = confirmExportOverwrite(cmd, []string{existing}, bufio.NewReader(cmd.Root().Reader))
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exist")
		assert.Contains(t, err.Error(), "--yes")
	})

	t.Run("TTY prompt accepts 'y'", func(t *testing.T) {
		mockTTY(t)

		var proceed bool

		var err error

		existing := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, os.WriteFile(existing, []byte("{}"), 0o600))

		reader := &fakeTTY{}
		_, _ = reader.WriteString("y\n")

		runWithCmd(t, exportFlags(), reader, &fakeTTY{}, &fakeTTY{},
			nil, func(cmd *cli.Command) {
				proceed, err = confirmExportOverwrite(cmd, []string{existing}, bufio.NewReader(cmd.Root().Reader))
			})

		require.NoError(t, err)
		assert.True(t, proceed)
	})

	t.Run("TTY prompt declines on 'n'", func(t *testing.T) {
		mockTTY(t)

		var proceed bool

		var err error

		existing := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, os.WriteFile(existing, []byte("{}"), 0o600))

		reader := &fakeTTY{}
		_, _ = reader.WriteString("n\n")

		runWithCmd(t, exportFlags(), reader, &fakeTTY{}, &fakeTTY{},
			nil, func(cmd *cli.Command) {
				proceed, err = confirmExportOverwrite(cmd, []string{existing}, bufio.NewReader(cmd.Root().Reader))
			})

		require.NoError(t, err)
		assert.False(t, proceed)
	})
}

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
// newReAnchorResolver
// =============================================================================

func TestNewReAnchorResolver(t *testing.T) {
	t.Parallel()

	t.Run("unknown service errors", func(t *testing.T) {
		t.Parallel()

		resolver := newReAnchorResolver(t.Context(), map[staging.Service]reAnchorSpec{})

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

		resolver := newReAnchorResolver(t.Context(), map[staging.Service]reAnchorSpec{
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

		resolver := newReAnchorResolver(t.Context(), map[staging.Service]reAnchorSpec{
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

		resolver := newReAnchorResolver(t.Context(), map[staging.Service]reAnchorSpec{
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
