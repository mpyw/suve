// Shared test doubles for the in-package tests (TTY fakes, an erroring reader,
// a generic leaf-command runner). They belong to no single command file.
//declscope:namespace fake
//declscope:package // consumed by the export and import in-package tests

package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

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

// fakeFailingReader always fails, so a stdin read surfaces a non-EOF error.
type fakeFailingReader struct{}

func (fakeFailingReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

// fakeFullStrategy is a sentinel staging.FullStrategy. It embeds the interface so
// it satisfies the type without implementing every method; the re-anchor resolver
// only stores and returns it, never invoking a method.
type fakeFullStrategy struct {
	staging.FullStrategy
}

// fakeIsTTY forces terminal.IsTTY to report a terminal for the duration of the
// test. It mutates a global, so callers must not run in parallel.
func fakeIsTTY(t *testing.T) {
	t.Helper()

	orig := terminal.IsTTY

	t.Cleanup(func() { terminal.IsTTY = orig })

	terminal.IsTTY = func(uintptr) bool { return true }
}

// runFakeLeafCommand wires reader/writer/errWriter onto a root app and parses args into a
// leaf command carrying flags, then invokes fn from inside the leaf's action so
// fn sees fully-parsed flag values and a resolvable cmd.Root().
func runFakeLeafCommand(
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
