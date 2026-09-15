// These are export.go's tests: exportPassphrase and confirmExportOverwrite are
// private to the export namespace.
//declscope:namespace export

package cli

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

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
